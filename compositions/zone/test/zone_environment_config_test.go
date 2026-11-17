package test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// renderEnvironmentConfigData returns data of the platform-apis-zone EnvironmentConfig that the
// chart renders with the given --set overrides.
func renderEnvironmentConfigData(t *testing.T, values ...string) map[string]any {
	t.Helper()
	args := []string{"template", "test-release", chartDir}
	for _, v := range values {
		args = append(args, "--set", v)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, doc := range strings.Split(string(out), "\n---") {
		var obj struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Data map[string]any `yaml:"data"`
		}
		if yaml.Unmarshal([]byte(doc), &obj) == nil && obj.Kind == "EnvironmentConfig" && obj.Metadata.Name == "platform-apis-zone" {
			return obj.Data
		}
	}
	t.Fatal("EnvironmentConfig platform-apis-zone not found in rendered chart")
	return nil
}

// TestZoneEnvironmentConfigGranularEgressExclude guards the list shape of granularEgressExclude.
// Printing the value directly renders a Go slice as "[bar foo]", which YAML reads as a single
// element "bar foo", so no Zone would ever match the exclusion.
func TestZoneEnvironmentConfigGranularEgressExclude(t *testing.T) {
	t.Parallel()

	t.Run("every zone name is its own element", func(t *testing.T) {
		t.Parallel()
		data := renderEnvironmentConfigData(t,
			"zone.environmentConfig.granularEgress=true",
			"zone.environmentConfig.granularEgressExclude[0]=bar",
			"zone.environmentConfig.granularEgressExclude[1]=foo")
		want := []any{"bar", "foo"}
		if got := data["granularEgressExclude"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("granularEgressExclude = %#v, want %#v", got, want)
		}
	})

	t.Run("the default is an empty list", func(t *testing.T) {
		t.Parallel()
		data := renderEnvironmentConfigData(t)
		got, ok := data["granularEgressExclude"].([]any)
		if !ok || len(got) != 0 {
			t.Fatalf("granularEgressExclude = %#v, want an empty list", data["granularEgressExclude"])
		}
	})
}

// renderInfralibZoneLabels returns metadata.labels of the infralib Zone that the chart renders
// with the given --set overrides.
func renderInfralibZoneLabels(t *testing.T, values ...string) map[string]any {
	t.Helper()
	args := []string{"template", "test-release", chartDir}
	for _, v := range values {
		args = append(args, "--set", v)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, doc := range strings.Split(string(out), "\n---") {
		var obj struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name   string         `yaml:"name"`
				Labels map[string]any `yaml:"labels"`
			} `yaml:"metadata"`
		}
		if yaml.Unmarshal([]byte(doc), &obj) == nil && obj.Kind == "Zone" && obj.Metadata.Name == "infralib" {
			return obj.Metadata.Labels
		}
	}
	t.Fatal("Zone infralib not found in rendered chart")
	return nil
}

// TestInfralibZoneLabels covers zone.infralibZone.labels. A label value has to be a string, and
// --set turns "true" into a boolean, so the template must quote it.
func TestInfralibZoneLabels(t *testing.T) {
	t.Parallel()

	t.Run("labels are rendered as strings", func(t *testing.T) {
		t.Parallel()
		labels := renderInfralibZoneLabels(t,
			`zone.infralibZone.labels.tenancy\.entigo\.com/default-zone=true`,
			"zone.infralibZone.labels.team=platform")
		want := map[string]any{"tenancy.entigo.com/default-zone": "true", "team": "platform"}
		if !reflect.DeepEqual(labels, want) {
			t.Fatalf("labels = %#v, want %#v", labels, want)
		}
	})

	t.Run("the default renders no labels", func(t *testing.T) {
		t.Parallel()
		if labels := renderInfralibZoneLabels(t); len(labels) != 0 {
			t.Fatalf("labels = %#v, want none", labels)
		}
	})
}
