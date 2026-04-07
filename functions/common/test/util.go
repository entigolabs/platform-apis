package test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/crossplane/function-sdk-go"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func IgnoreFields(fields ...string) cmp.Option {
	fieldsMap := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		fieldsMap[fmt.Sprintf(`["%s"]`, field)] = nil
	}
	return cmp.FilterPath(func(p cmp.Path) bool {
		vx := p.Last().String()
		_, ignored := fieldsMap[vx]
		return ignored
	}, cmp.Ignore())
}

func EnvironmentConfigResourceWithData(name string, data map[string]interface{}) *fnv1.Resources {
	resourceStruct, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": base.EnvironmentApiVersion,
		"kind":       base.EnvironmentKind,
		"metadata": map[string]interface{}{
			"name": name,
		},
		"data": data,
	})
	if err != nil {
		panic(err)
	}
	return &fnv1.Resources{
		Items: []*fnv1.Resource{
			{
				Resource: resourceStruct,
			},
		},
	}
}

func KMSKeyResource(name, namespace, arnSuffix string) *fnv1.Resources {
	resourceStruct, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": base.KMSKeyApiVersion,
		"kind":       base.KMSKeyKind,
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				"crossplane.io/external-name": "arn:aws:kms:eu-north-1:111111111111:key/" + arnSuffix,
			},
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"forProvider": map[string]interface{}{
				"region": "eu-north-1",
			},
		},
		"status": map[string]interface{}{
			"atProvider": map[string]interface{}{
				"arn":    "arn:aws:kms:eu-north-1:111111111111:key/" + arnSuffix,
				"region": "eu-north-1",
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return &fnv1.Resources{
		Items: []*fnv1.Resource{
			{
				Resource: resourceStruct,
			},
		},
	}
}

func Namespace(name, zone string) *fnv1.Resources {
	resourceStruct, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": name,
			"labels": map[string]interface{}{
				base.TenancyZoneLabel: zone,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return &fnv1.Resources{
		Items: []*fnv1.Resource{
			{
				Resource: resourceStruct,
			},
		},
	}
}

func Zone(name string) *fnv1.Resources {
	return ZoneWithMetadata(name, nil, nil)
}

func ZoneWithMetadata(name string, labels, annotations map[string]interface{}) *fnv1.Resources {
	metadata := map[string]interface{}{
		"name": name,
	}
	if len(labels) > 0 {
		metadata["labels"] = labels
	}
	if len(annotations) > 0 {
		metadata["annotations"] = annotations
	}
	resourceStruct, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": base.TenancyApiVersion,
		"kind":       base.ZoneKey,
		"metadata":   metadata,
	})
	if err != nil {
		panic(err)
	}
	return &fnv1.Resources{
		Items: []*fnv1.Resource{
			{
				Resource: resourceStruct,
			},
		},
	}
}

type Args struct {
	Ctx context.Context
	Req *fnv1.RunFunctionRequest
}

type Want struct {
	Rsp *fnv1.RunFunctionResponse
	Err error
}

type Case struct {
	Reason string
	Args   Args
	Want   Want
}

func RunFunctionCases(t *testing.T, serviceFn func() base.GroupService, cases map[string]Case, ignoredFields ...string) {
	log, err := function.NewLogger(true)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
		return
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			service := serviceFn()
			service.SetLogger(log)
			f := base.NewFunction(log, service, "")
			rsp, err := f.RunFunction(tc.Args.Ctx, tc.Args.Req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
				t.Errorf("Response failure: %v", rsp.GetResults()[0].GetMessage())
				return
			}
			if diff := cmp.Diff(tc.Want.Rsp, rsp, protocmp.Transform(), IgnoreFields(ignoredFields...)); diff != "" {
				//Can be used to print the desired resources
				for key, value := range rsp.GetDesired().GetResources() {
					fmt.Println(key)
					rspResource, _ := json.Marshal(value.Resource)
					fmt.Println(string(rspResource))
				}
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.Reason, diff)
			}

			if diff := cmp.Diff(tc.Want.Err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.Reason, diff)
			}
		})
	}
}

func AddEnvironmentConfig(cases map[string]Case, name string, data map[string]interface{}) {
	env := EnvironmentConfigResourceWithData(name, data)
	required := base.RequiredEnvironmentConfig(name)
	for _, testCase := range cases {
		if testCase.Args.Req.RequiredResources == nil {
			testCase.Args.Req.RequiredResources = make(map[string]*fnv1.Resources)
		}
		args := testCase.Args.Req.RequiredResources
		if _, found := args[base.EnvironmentKey]; !found {
			args[base.EnvironmentKey] = env
		}

		if testCase.Want.Rsp.Requirements == nil {
			testCase.Want.Rsp.Requirements = &fnv1.Requirements{}
		}
		if testCase.Want.Rsp.Requirements.Resources == nil {
			testCase.Want.Rsp.Requirements.Resources = make(map[string]*fnv1.ResourceSelector)
		}
		requirements := testCase.Want.Rsp.Requirements.Resources
		if _, found := requirements[base.EnvironmentKey]; !found {
			requirements[base.EnvironmentKey] = required
		}
	}
}

func AddZoneResources(cases map[string]Case, ns, zone string) {
	namespace := Namespace(ns, zone)
	requiredNs := base.RequiredNamespace(ns)
	zoneResource := Zone(zone)
	requiredZone := base.RequiredZone(zone)
	envConfig := EnvironmentConfigResourceWithData(base.ZoneEnvName, nil)
	requiredEnv := base.RequiredEnvironmentConfig(base.ZoneEnvName)
	for _, testCase := range cases {
		if testCase.Args.Req.RequiredResources == nil {
			testCase.Args.Req.RequiredResources = make(map[string]*fnv1.Resources)
		}
		args := testCase.Args.Req.RequiredResources
		if _, found := args[base.NamespaceKey]; !found {
			args[base.NamespaceKey] = namespace
		}
		if _, found := args[base.ZoneKey]; !found {
			args[base.ZoneKey] = zoneResource
		}
		if _, found := args[base.ZoneEnvKey]; !found {
			args[base.ZoneEnvKey] = envConfig
		}

		if testCase.Want.Rsp.Requirements == nil {
			testCase.Want.Rsp.Requirements = &fnv1.Requirements{}
		}
		if testCase.Want.Rsp.Requirements.Resources == nil {
			testCase.Want.Rsp.Requirements.Resources = make(map[string]*fnv1.ResourceSelector)
		}
		requirements := testCase.Want.Rsp.Requirements.Resources
		if _, found := requirements[base.NamespaceKey]; !found {
			requirements[base.NamespaceKey] = requiredNs
		}
		if _, found := requirements[base.ZoneKey]; !found {
			requirements[base.ZoneKey] = requiredZone
		}
		if _, found := requirements[base.ZoneEnvKey]; !found {
			requirements[base.ZoneEnvKey] = requiredEnv
		}
	}
}

func RequiredResource(data map[string]interface{}) resource.Required {
	return resource.Required{
		Resource: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"data": data,
			},
		},
	}
}

func RequiredNamespace(labels map[string]string) resource.Required {
	obj := unstructured.Unstructured{}
	obj.SetLabels(labels)
	return resource.Required{Resource: &obj}
}

func RequiredZoneObject(labels, annotations map[string]string) resource.Required {
	obj := unstructured.Unstructured{}
	obj.SetLabels(labels)
	obj.SetAnnotations(annotations)
	return resource.Required{Resource: &obj}
}

func RequiredEnvTags(tags map[string]string) resource.Required {
	envTags := base.EnvironmentTags{Tags: make(map[string]*string, len(tags))}
	for k, v := range tags {
		envTags.Tags[k] = base.StringPtr(v)
	}
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&envTags)
	if err != nil {
		panic(fmt.Sprintf("RequiredEnvTags: ToUnstructured failed: %v", err))
	}
	return RequiredResource(data)
}

func CompositeResource(name, namespace string, labels, annotations map[string]string) *resource.Composite {
	obj := unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(labels)
	obj.SetAnnotations(annotations)
	return &resource.Composite{
		Resource: &composite.Unstructured{Unstructured: obj},
	}
}
