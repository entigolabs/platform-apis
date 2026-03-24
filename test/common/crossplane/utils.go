package crossplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane/v2/cmd/crank/render"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// StartCustomFunction starts written in golang function
func StartCustomFunction(t *testing.T, funcPath string, port string) {
	t.Helper()

	cmd := exec.Command("go", "run", ".", "--insecure", "--debug")
	cmd.Dir = funcPath
	// Put the process in its own group so the whole tree (go run + compiled binary) is killed on cleanup.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	require.NoError(t, err, "Cannot start custom function")

	t.Cleanup(func() {
		if cmd.Process != nil {
			t.Log("Killing custom function...")
			// Kill the entire process group (go run + its compiled binary child).
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	})

	address := "localhost:" + port

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		return false
	}, 10*time.Second, 500*time.Millisecond, "Function doesn't bind to port %s", port)

	t.Logf("Custom function started using port %s", port)
}

// Ptr returns a pointer to the given string value. Use it to pass optional *string args.
func Ptr(s string) *string { return &s }

// CrossplaneRender is crossplane rendering function. Renders resources using definition, composition and functions
// param extraResourcesFile: optional path to extra resources YAML such as EnvironmentConfig (passed as -e); pass nil to omit
// param observedFile: optional path to observed resources YAML (passed as -o); pass nil to omit
func CrossplaneRender(t *testing.T, testResource, composition, functions string, extraResourcesFile, observedFile *string) []*unstructured.Unstructured {
	t.Helper()
	cmd := &render.Cmd{}

	var out bytes.Buffer
	parser, err := kong.New(cmd,
		kong.Writers(&out, &out),
		kong.BindTo(afero.NewOsFs(), (*afero.Fs)(nil)),
	)
	require.NoError(t, err, "Kong parser initialization failed")

	args := []string{testResource, composition, functions, "-r", "-x"}
	if extraResourcesFile != nil {
		args = append(args, "-e", *extraResourcesFile)
	}
	if observedFile != nil {
		args = append(args, "-o", *observedFile)
	}

	kongCtx, err := parser.Parse(args)
	require.NoError(t, err, "Arguments parsing error")

	err = cmd.Run(kongCtx, logging.NewNopLogger())
	require.NoError(t, err, "Render error: could not render composition")

	var resources []*unstructured.Unstructured
	for _, resString := range strings.Split(out.String(), "---") {
		if strings.TrimSpace(resString) == "" {
			continue
		}
		obj := &unstructured.Unstructured{}
		require.NoError(t, yaml.Unmarshal([]byte(resString), &obj.Object))
		resources = append(resources, obj)
	}
	return resources
}

// AssertResourceCount validates count of specified rendered resource.
func AssertResourceCount(t *testing.T, resources []*unstructured.Unstructured, kind string, expected int) {
	t.Helper()
	count := 0
	for _, res := range resources {
		if res.GetKind() == kind {
			count++
		}
	}
	assert.Equal(t, expected, count, "Resources count mismatch. Kind: %s, Expected: %d, Actual: %d", kind, expected, count)
	t.Logf("Resource count OK. Kind: %s, Count: %d", kind, count)
}

// AssertResourceReady validates that rendered resource is ready (condition type Ready has status True)
func AssertResourceReady(t *testing.T, resources []*unstructured.Unstructured, kind string, apiVersion string) {
	t.Helper()
	for _, res := range resources {
		if res.GetKind() == kind && res.GetAPIVersion() == apiVersion {
			conditions, _, _ := unstructured.NestedSlice(res.Object, "status", "conditions")
			for _, c := range conditions {
				condition := c.(map[string]interface{})
				if condition["type"] == "Ready" {
					assert.True(t, condition["status"] == "True")
					t.Logf("Resource Ready. Kind: %s", kind)
					return
				}
			}
			t.Errorf("Resource %s not ready", kind)
			return
		}
	}
	t.Errorf("Resource %s not found", kind)
}

// AssertFieldValues asserts that at least one resource of kind+apiVersion exists
// where every field in fields matches its expected value.
// Use "*" as the expected value to assert the field exists and is non-null.
// Uses gjson path syntax: "metadata.name", "metadata.ownerReferences.0.apiVersion"
func AssertFieldValues(t *testing.T, resources []*unstructured.Unstructured, kind, apiVersion string, fields map[string]string) {
	t.Helper()
	for _, res := range resources {
		if res.GetKind() != kind || res.GetAPIVersion() != apiVersion {
			continue
		}
		data, _ := json.Marshal(res.Object)
		j := string(data)
		match := true
		for path, expected := range fields {
			val := gjson.Get(j, path)
			if expected == "*" {
				if !val.Exists() || val.Type == gjson.Null {
					match = false
					break
				}
			} else if val.String() != expected {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		for path, expected := range fields {
			val := gjson.Get(j, path)
			if expected == "*" {
				assert.True(t, val.Exists() && val.Type != gjson.Null, "Field %s is missing or null in %s/%s", path, apiVersion, kind)
			} else {
				assert.Equal(t, expected, val.String(), "Field %s mismatch in %s/%s", path, apiVersion, kind)
			}
		}
		t.Logf("Fields OK. ApiVersion: %s. Kind:%s. Name: %s", apiVersion, kind, res.GetName())
		return
	}
	t.Errorf("No %s/%s found matching all provided fields", apiVersion, kind)
}

// MockResource returns a deep copy of the first resource matching kind+apiVersion,
// optionally marking it ready and applying field overrides.
// fieldChanges keys use dot-separated paths of any depth: "spec.forProvider.region".
// Pass nil for fieldChanges to skip field overrides.
func MockResource(t *testing.T, resources []*unstructured.Unstructured, kind, apiVersion string, makeReady bool, fieldChanges map[string]interface{}) *unstructured.Unstructured {
	t.Helper()
	for _, res := range resources {
		if res.GetKind() != kind || res.GetAPIVersion() != apiVersion {
			continue
		}
		mockedResource := res.DeepCopy()
		if makeReady {
			conditions := []interface{}{
				map[string]interface{}{"type": "Synced", "status": "True"},
				map[string]interface{}{"type": "Ready", "status": "True"},
			}
			_ = unstructured.SetNestedSlice(mockedResource.Object, conditions, "status", "conditions")
		}
		for path, value := range fieldChanges {
			err := unstructured.SetNestedField(mockedResource.Object, value, strings.Split(path, ".")...)
			require.NoError(t, err, "Can not mock field %s", path)
		}
		return mockedResource
	}
	t.Fatalf("Resource %s/%s for mocking not found", apiVersion, kind)
	return nil
}

// AppendToResources appends resources to observed state resources
func AppendToResources(t *testing.T, filename string, resources ...*unstructured.Unstructured) {
	t.Helper()
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err, "Can not open file %s", filename)
	defer file.Close()

	for _, res := range resources {
		data, err := yaml.Marshal(res.Object)
		require.NoError(t, err)
		file.Write([]byte("---\n"))
		file.Write(data)
	}
}

// AppendYamlToResources appends resources in YAML files to observed state resources
func AppendYamlToResources(t *testing.T, sourceFilename string, destFilename string) {
	t.Helper()

	data, err := os.ReadFile(sourceFilename)
	require.NoError(t, err, "Can not read file %s", sourceFilename)

	file, err := os.OpenFile(destFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err, "Can not open target file %s", destFilename)
	defer file.Close()

	file.Write([]byte("\n---\n"))
	file.Write(data)
}

func ParseYamlFileToUnstructured(t *testing.T, filename string) []*unstructured.Unstructured {
	t.Helper()

	data, err := os.ReadFile(filename)
	require.NoError(t, err, "Can not read file %s", filename)

	var resources []*unstructured.Unstructured

	yamlStrings := strings.Split(string(data), "---")

	for _, yamlString := range yamlStrings {
		yamlString = strings.TrimSpace(yamlString)
		if yamlString == "" {
			continue
		}

		obj := &unstructured.Unstructured{}

		err := yaml.Unmarshal([]byte(yamlString), &obj.Object)
		require.NoError(t, err, "Can not parse from filename %s:\n%s", filename, yamlString)

		resources = append(resources, obj)
	}

	return resources
}

//TODO: Used in Kafka only. Remove functions below when Kafka transitioned to golang custom function

// GenerateFunctionsConfig generates a functions config YAML from helm values,
// appending the dev functions overlay. Returns the path to the generated temp file.
func GenerateFunctionsConfig(t *testing.T, helmValuesPath, devFunctionsPath string) string {
	t.Helper()

	data, err := os.ReadFile(helmValuesPath)
	require.NoError(t, err, "cannot read helm values")

	var values struct {
		Functions map[string]struct {
			Install bool   `yaml:"install"`
			Image   string `yaml:"image"`
			Tag     string `yaml:"tag"`
		} `yaml:"functions"`
	}
	require.NoError(t, yaml.Unmarshal(data, &values), "cannot parse helm values")

	outputPath := filepath.Join(t.TempDir(), "functions.yaml")
	f, err := os.Create(outputPath)
	require.NoError(t, err, "cannot create functions config file")
	defer f.Close()

	for name, fn := range values.Functions {
		if !fn.Install {
			continue
		}
		fmt.Fprintf(f, "---\napiVersion: pkg.crossplane.io/v1beta1\nkind: Function\nmetadata:\n  name: %s\nspec:\n  package: %s:%s\n", name, fn.Image, fn.Tag)
	}

	dev, err := os.ReadFile(devFunctionsPath)
	require.NoError(t, err, "cannot read dev functions")
	fmt.Fprint(f, "---\n")
	_, _ = f.Write(dev)

	return outputPath
}

// RemovePipelineStep copies src to dst and removes the named pipeline step using yq.
// Use this when the composition contains Go template syntax that Go's YAML parser cannot handle.
func RemovePipelineStep(t *testing.T, srcPath, dstPath, stepName string) {
	t.Helper()
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err, "cannot read %s", srcPath)
	require.NoError(t, os.WriteFile(dstPath, data, 0644), "cannot write %s", dstPath)
	cmd := exec.Command("yq", "-i", fmt.Sprintf("del(.spec.pipeline[] | select(.step == %q))", stepName), dstPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "yq failed: %s", string(out))
}
