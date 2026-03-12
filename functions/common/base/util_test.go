package base

import (
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetCrossplaneReadyStatus(t *testing.T) {
	cases := map[string]struct {
		observed *composed.Unstructured
		want     resource.Ready
	}{
		"ReadyTrue": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"status": map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"type": "Ready", "status": "True"},
							},
						},
					},
				},
			},
			want: resource.ReadyTrue,
		},
		"ReadyFalse": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"status": map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"type": "Ready", "status": "False"},
							},
						},
					},
				},
			},
			want: resource.ReadyFalse,
		},
		"SyncedFalseAfterReadyTrue": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"status": map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"type": "Ready", "status": "True"},
								map[string]interface{}{"type": "Synced", "status": "False"},
							},
						},
					},
				},
			},
			want: resource.ReadyFalse,
		},
		"SyncedFalseBeforeReadyTrue": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"status": map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"type": "Synced", "status": "False"},
								map[string]interface{}{"type": "Ready", "status": "True"},
							},
						},
					},
				},
			},
			want: resource.ReadyFalse,
		},
		"NoConditions_K8sNative_Ready": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
					},
				},
			},
			want: resource.ReadyTrue,
		},
		"NoConditions_Upbound_NotReady": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind":       "Instance",
					},
				},
			},
			want: resource.ReadyFalse,
		},
		"MalformedCondition_SkipAndDefault": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"status": map[string]interface{}{
							"conditions": []interface{}{
								"this-is-not-a-map",
							},
						},
					},
				},
			},
			want: resource.ReadyTrue,
		},
		"UnknownConditionType_HitDefault": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"status": map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"type": "SomeOtherType", "status": "True"},
							},
						},
					},
				},
			},
			want: resource.ReadyFalse,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := GetCrossplaneReadyStatus(tc.observed); !cmp.Equal(got, tc.want, cmpopts.EquateEmpty()) {
				t.Errorf("%s: GetCrossplaneReadyStatus() = %v, want %v", name, got, tc.want)
			}
		})
	}
}

func TestGenerateEligibleKubernetesName(t *testing.T) {
	cases := map[string]struct {
		input string
		limit int
		want  string
	}{
		"AlreadyValid": {
			input: "my-resource",
			limit: 253,
			want:  "my-resource",
		},
		"UppercaseConverted": {
			input: "MyResource",
			limit: 253,
			want:  "myresource",
		},
		"SpecialCharsReplacedWithDash": {
			input: "my_resource.name",
			limit: 253,
			want:  "my-resource-name",
		},
		"ConsecutiveSpecialCharsCollapsed": {
			input: "my__resource..name",
			limit: 253,
			want:  "my-resource-name",
		},
		"LeadingNonAlphaStripped": {
			input: "123abc",
			limit: 253,
			want:  "abc",
		},
		"TrailingNonAlphanumericStripped": {
			input: "abc---",
			limit: 253,
			want:  "abc",
		},
		"TruncatedToLimit": {
			input: "abcdefghij",
			limit: 5,
			want:  "abcde",
		},
		"TruncationTrailingDashCleaned": {
			input: "abcd-fghij",
			limit: 5,
			want:  "abcd",
		},
		"UnicodeTransliterated": {
			input: "über-service",
			limit: 253,
			want:  "uber-service",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := GenerateEligibleKubernetesName(tc.input, tc.limit)
			if got != tc.want {
				t.Errorf("GenerateEligibleKubernetesName(%q, %d) = %q, want %q", tc.input, tc.limit, got, tc.want)
			}
		})
	}
}

// testEnvConfig is a minimal Validatable used by GetEnvironment tests.
type testEnvConfig struct {
	Name   string   `json:"name"`
	Region string   `json:"region"`
	Items  []string `json:"items"`
}

func (c *testEnvConfig) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

func requiredResource(data map[string]interface{}) resource.Required {
	return resource.Required{
		Resource: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"data": data,
			},
		},
	}
}

func TestGetEnvironment(t *testing.T) {
	cases := map[string]struct {
		resources []resource.Required
		wantErr   bool
		want      testEnvConfig
	}{
		"EmptyResourcesReturnsError": {
			resources: []resource.Required{},
			wantErr:   true,
		},
		"SingleResourcePopulatesFields": {
			resources: []resource.Required{
				requiredResource(map[string]interface{}{"name": "foo", "region": "us-east-1"}),
			},
			want: testEnvConfig{Name: "foo", Region: "us-east-1"},
		},
		"MultipleResourcesMergeNonOverlappingKeys": {
			resources: []resource.Required{
				requiredResource(map[string]interface{}{"name": "foo"}),
				requiredResource(map[string]interface{}{"region": "eu-west-1"}),
			},
			want: testEnvConfig{Name: "foo", Region: "eu-west-1"},
		},
		"MultipleResourcesAppendSlices": {
			resources: []resource.Required{
				requiredResource(map[string]interface{}{"name": "foo", "items": []interface{}{"a"}}),
				requiredResource(map[string]interface{}{"items": []interface{}{"b"}}),
			},
			want: testEnvConfig{Name: "foo", Items: []string{"a", "b"}},
		},
		"ValidationFailureReturnsError": {
			resources: []resource.Required{
				requiredResource(map[string]interface{}{"region": "us-east-1"}),
			},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			required := map[string][]resource.Required{"env": tc.resources}
			got := &testEnvConfig{}
			err := GetEnvironment("env", required, got)
			if (err != nil) != tc.wantErr {
				t.Errorf("GetEnvironment() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr {
				if diff := cmp.Diff(tc.want, *got, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("GetEnvironment() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestGetTenancyZone(t *testing.T) {
	log := logging.NewNopLogger()

	namespaceResource := func(labels map[string]interface{}) resource.Required {
		return resource.Required{
			Resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name":   "test-ns",
						"labels": labels,
					},
				},
			},
		}
	}

	cases := map[string]struct {
		required map[string][]resource.Required
		want     string
	}{
		"NoNamespaceKeyReturnsEmpty": {
			required: map[string][]resource.Required{},
			want:     "",
		},
		"NamespaceWithNoLabelsReturnsEmpty": {
			required: map[string][]resource.Required{
				NamespaceKey: {namespaceResource(map[string]interface{}{})},
			},
			want: "",
		},
		"NamespaceWithoutZoneLabelReturnsEmpty": {
			required: map[string][]resource.Required{
				NamespaceKey: {namespaceResource(map[string]interface{}{"other-label": "value"})},
			},
			want: "",
		},
		"NamespaceWithZoneLabelReturnsZone": {
			required: map[string][]resource.Required{
				NamespaceKey: {namespaceResource(map[string]interface{}{TenancyZoneLabel: "my-zone"})},
			},
			want: "my-zone",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := GetTenancyZone(tc.required, log)
			if got != tc.want {
				t.Errorf("GetTenancyZone() = %q, want %q", got, tc.want)
			}
		})
	}
}
