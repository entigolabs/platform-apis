package base_test

import (
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	basetest "github.com/entigolabs/function-base/test"
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
			if got := base.GetCrossplaneReadyStatus(tc.observed); !cmp.Equal(got, tc.want, cmpopts.EquateEmpty()) {
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
			got := base.GenerateEligibleKubernetesName(tc.input, tc.limit)
			if got != tc.want {
				t.Errorf("GenerateEligibleKubernetesName(%q, %d) = %q, want %q", tc.input, tc.limit, got, tc.want)
			}
		})
	}
}

// testEnvConfig is a minimal base.Validatable used by GetEnvironment tests.
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
				basetest.RequiredResource(map[string]interface{}{"name": "foo", "region": "us-east-1"}),
			},
			want: testEnvConfig{Name: "foo", Region: "us-east-1"},
		},
		"MultipleResourcesMergeNonOverlappingKeys": {
			resources: []resource.Required{
				basetest.RequiredResource(map[string]interface{}{"name": "foo"}),
				basetest.RequiredResource(map[string]interface{}{"region": "eu-west-1"}),
			},
			want: testEnvConfig{Name: "foo", Region: "eu-west-1"},
		},
		"MultipleResourcesAppendSlices": {
			resources: []resource.Required{
				basetest.RequiredResource(map[string]interface{}{"name": "foo", "items": []interface{}{"a"}}),
				basetest.RequiredResource(map[string]interface{}{"items": []interface{}{"b"}}),
			},
			want: testEnvConfig{Name: "foo", Items: []string{"a", "b"}},
		},
		"ValidationFailureReturnsError": {
			resources: []resource.Required{
				basetest.RequiredResource(map[string]interface{}{"region": "us-east-1"}),
			},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			required := map[string][]resource.Required{"env": tc.resources}
			got := &testEnvConfig{}
			err := base.GetEnvironment("env", required, got)
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
				base.NamespaceKey: {basetest.RequiredNamespace(nil)},
			},
			want: "",
		},
		"NamespaceWithoutZoneLabelReturnsEmpty": {
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{"other-label": "value"})},
			},
			want: "",
		},
		"NamespaceWithZoneLabelReturnsZone": {
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: "my-zone"})},
			},
			want: "my-zone",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := base.GetTenancyZone(tc.required, log)
			if got != tc.want {
				t.Errorf("GetTenancyZone() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetResourceTags(t *testing.T) {
	log := logging.NewNopLogger()
	const zone = "my-zone"

	cases := map[string]struct {
		composite  *resource.Composite
		required   map[string][]resource.Required
		wantTags   map[string]string
		wantLabels map[string]string
		wantZone   string
	}{
		// No zone: only CR labels and annotations are used.
		"NoZone_CRLabelsAndAnnotations": {
			composite: basetest.CompositeResource("cr", "ns",
				map[string]string{base.TagsPrefix + "CRTag": "cr-val"},
				map[string]string{base.TagsPrefix + "CRAnnotation": "cr-ann-val"},
			),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(nil)},
			},
			wantZone:   "",
			wantTags:   map[string]string{"CRTag": "cr-val", "CRAnnotation": "cr-ann-val"},
			wantLabels: map[string]string{base.TagsPrefix + "CRTag": "cr-val"},
		},
		// Zone env config is the lowest-priority source.
		"WithZone_ZoneEnvConfigBase": {
			composite: basetest.CompositeResource("cr", "ns", nil, nil),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: zone})},
				base.ZoneEnvKey:   {basetest.RequiredEnvTags(map[string]string{"EnvTag": "from-env-config"})},
				base.ZoneKey:      {basetest.RequiredZoneObject(nil, nil)},
			},
			wantZone:   zone,
			wantTags:   map[string]string{"EnvTag": "from-env-config"},
			wantLabels: map[string]string{},
		},
		// Zone label overrides zone env config for the same key.
		"WithZone_ZoneLabelOverridesEnvConfig": {
			composite: basetest.CompositeResource("cr", "ns", nil, nil),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: zone})},
				base.ZoneEnvKey:   {basetest.RequiredEnvTags(map[string]string{"Env": "from-env-config"})},
				base.ZoneKey:      {basetest.RequiredZoneObject(map[string]string{base.TagsPrefix + "Env": "from-zone-label"}, nil)},
			},
			wantZone:   zone,
			wantTags:   map[string]string{"Env": "from-zone-label"},
			wantLabels: map[string]string{base.TagsPrefix + "Env": "from-zone-label"},
		},
		// CR label overrides zone label for the same key.
		"WithZone_CRLabelOverridesZoneLabel": {
			composite: basetest.CompositeResource("cr", "ns",
				map[string]string{base.TagsPrefix + "Env": "from-cr-label"},
				nil,
			),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: zone})},
				base.ZoneKey:      {basetest.RequiredZoneObject(map[string]string{base.TagsPrefix + "Env": "from-zone-label"}, nil)},
			},
			wantZone:   zone,
			wantTags:   map[string]string{"Env": "from-cr-label"},
			wantLabels: map[string]string{base.TagsPrefix + "Env": "from-cr-label"},
		},
		// CR annotation overrides CR label in tags; the original label value is preserved in labels.
		"WithZone_CRAnnotationOverridesCRLabelInTags": {
			composite: basetest.CompositeResource("cr", "ns",
				map[string]string{base.TagsPrefix + "Env": "from-cr-label"},
				map[string]string{base.TagsPrefix + "Env": "from-cr-annotation"},
			),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: zone})},
				base.ZoneKey:      {basetest.RequiredZoneObject(nil, nil)},
			},
			wantZone:   zone,
			wantTags:   map[string]string{"Env": "from-cr-annotation"},
			wantLabels: map[string]string{base.TagsPrefix + "Env": "from-cr-label"},
		},
		// Full hierarchy: each level contributes a distinct key.
		"WithZone_FullHierarchy": {
			composite: basetest.CompositeResource("cr", "ns",
				map[string]string{base.TagsPrefix + "CRTag": "from-cr"},
				nil,
			),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: zone})},
				base.ZoneEnvKey:   {basetest.RequiredEnvTags(map[string]string{"EnvTag": "from-env-config"})},
				base.ZoneKey:      {basetest.RequiredZoneObject(map[string]string{base.TagsPrefix + "ZoneTag": "from-zone"}, nil)},
			},
			wantZone: zone,
			wantTags: map[string]string{
				"EnvTag":  "from-env-config",
				"ZoneTag": "from-zone",
				"CRTag":   "from-cr",
			},
			wantLabels: map[string]string{
				base.TagsPrefix + "ZoneTag": "from-zone",
				base.TagsPrefix + "CRTag":   "from-cr",
			},
		},
		// Zone found in namespace but Zone object absent from required: env config tags used, no zone labels.
		"WithZone_ZoneObjectMissing": {
			composite: basetest.CompositeResource("cr", "ns", nil, nil),
			required: map[string][]resource.Required{
				base.NamespaceKey: {basetest.RequiredNamespace(map[string]string{base.TenancyZoneLabel: zone})},
				base.ZoneEnvKey:   {basetest.RequiredEnvTags(map[string]string{"EnvTag": "from-env-config"})},
			},
			wantZone:   zone,
			wantTags:   map[string]string{"EnvTag": "from-env-config"},
			wantLabels: map[string]string{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := base.GetResourceTags(log, tc.composite, tc.required)
			if got.Zone != tc.wantZone {
				t.Errorf("GetResourceTags() Zone = %q, want %q", got.Zone, tc.wantZone)
			}
			if diff := cmp.Diff(tc.wantTags, got.Tags, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("GetResourceTags() Tags mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantLabels, got.Labels, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("GetResourceTags() Labels mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetZoneTags(t *testing.T) {
	const zoneName = "my-zone"

	cases := map[string]struct {
		compositeName        string
		compositeLabels      map[string]string
		compositeAnnotations map[string]string
		wantZone             string
		wantTags             map[string]string
		wantLabels           map[string]string
	}{
		// Zone label overrides env config for the same key.
		"ZoneLabelsOnly": {
			compositeName:   zoneName,
			compositeLabels: map[string]string{base.TagsPrefix + "Env": "from-label"},
			wantZone:        zoneName,
			wantTags:        map[string]string{"Env": "from-label"},
			wantLabels:      map[string]string{base.TagsPrefix + "Env": "from-label"},
		},
		// Zone annotation overrides zone label in tags; the original label value is preserved in labels.
		"ZoneAnnotationOverridesLabelInTags": {
			compositeName:        zoneName,
			compositeLabels:      map[string]string{base.TagsPrefix + "Env": "from-label"},
			compositeAnnotations: map[string]string{base.TagsPrefix + "Env": "from-annotation"},
			wantZone:             zoneName,
			wantTags:             map[string]string{"Env": "from-annotation"},
			wantLabels:           map[string]string{base.TagsPrefix + "Env": "from-label"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := basetest.CompositeResource(tc.compositeName, "", tc.compositeLabels, tc.compositeAnnotations)
			got := base.GetZoneTags(cr)
			if got.Zone != tc.wantZone {
				t.Errorf("GetZoneTags() Zone = %q, want %q", got.Zone, tc.wantZone)
			}
			if diff := cmp.Diff(tc.wantTags, got.Tags, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("GetZoneTags() Tags mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantLabels, got.Labels, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("GetZoneTags() Labels mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
