package service

import (
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func oracleRequired(referencePools ...string) map[string][]resource.Required {
	obj := func(o map[string]any) []resource.Required {
		return []resource.Required{{Resource: &unstructured.Unstructured{Object: o}}}
	}
	required := map[string][]resource.Required{
		base.EnvironmentKey: obj(map[string]any{
			"apiVersion": "apiextensions.crossplane.io/v1beta1",
			"kind":       "EnvironmentConfig",
			"metadata":   map[string]any{"name": "platform-apis-zone"},
			"data": map[string]any{
				"cloud":             "oracle",
				"oracleProvider":    "crossplane-oracle",
				"argoCDNamespace":   "argocd",
				"cluster":           "oke",
				"securityGroup":     "node",
				"computeSubnetType": "private",
				"dataKMSKey":        "data",
				"podSecurity":       "baseline",
				"tags":              map[string]any{"env": "test"},
			},
		}),
		ClusterKey: obj(map[string]any{
			"apiVersion": "containerengine.oci.upbound.io/v1alpha1", "kind": "Cluster",
			"metadata": map[string]any{"name": "oke"},
			"status": map[string]any{"atProvider": map[string]any{
				"id": "ocid1.cluster.x", "compartmentId": "ocid1.compartment.x", "kubernetesVersion": "v1.34.1",
			}},
		}),
		SecurityGroupKey: obj(map[string]any{
			"apiVersion": "networking.oci.upbound.io/v1alpha1", "kind": "NetworkSecurityGroup",
			"metadata": map[string]any{"name": "node"},
			"status":   map[string]any{"atProvider": map[string]any{"id": "ocid1.nsg.node"}},
		}),
		KMSDataKeyKey: obj(map[string]any{
			"apiVersion": "kms.oci.upbound.io/v1alpha1", "kind": "Key",
			"metadata": map[string]any{"name": "data"},
			"status":   map[string]any{"atProvider": map[string]any{"id": "ocid1.key.data"}},
		}),
	}
	for _, s := range []struct{ name, subnetType, id string }{
		{"private-1", "private", "ocid1.subnet.private1"},
		{"private-0", "private", "ocid1.subnet.private0"},
		{"pod-0", "pod", "ocid1.subnet.pod0"},
	} {
		required[SubnetsKey] = append(required[SubnetsKey], obj(map[string]any{
			"apiVersion": "networking.oci.upbound.io/v1alpha1", "kind": "Subnet",
			"metadata": map[string]any{"name": s.name, "labels": map[string]any{subnetTypeLabel: s.subnetType}},
			"status":   map[string]any{"atProvider": map[string]any{"id": s.id}},
		})...)
	}
	for _, name := range referencePools {
		required[GetReferenceNodePoolKey(name)] = obj(map[string]any{
			"apiVersion": "containerengine.oci.upbound.io/v1alpha1", "kind": "NodePool",
			"metadata": map[string]any{"name": name},
			"status": map[string]any{"atProvider": map[string]any{
				"nodeSourceDetails": []any{map[string]any{"imageId": "ocid1.image." + name}},
				"nodeConfigDetails": []any{map[string]any{
					"placementConfigs": []any{
						map[string]any{"availabilityDomain": "AD-2", "subnetId": "ocid1.subnet.private0"},
						map[string]any{"availabilityDomain": "AD-1", "subnetId": "ocid1.subnet.private0"},
					},
					"nodePoolPodNetworkOptionDetails": []any{map[string]any{
						"cniType": "OCI_VCN_IP_NATIVE", "maxPodsPerNode": float64(31),
						"podSubnetIds": []any{"ocid1.subnet.pod0"}, "podNsgIds": []any{"ocid1.nsg.pod"},
					}},
				}},
			}},
		})
	}
	return required
}

func oracleZone(name string, requirements ...v1alpha1.Requirement) v1alpha1.Zone {
	return v1alpha1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.ZoneSpec{
			Pools: []v1alpha1.Pool{{Name: "default", Requirements: requirements}},
		},
	}
}

func flexRequirements() []v1alpha1.Requirement {
	return []v1alpha1.Requirement{
		{Key: "instance-type", Values: []string{"VM.Standard.E5.Flex"}},
		{Key: requirementOCPUs, Value: intstr.FromInt32(2)},
		{Key: requirementMemoryGB, Value: intstr.FromInt32(16)},
		{Key: "min-size", Value: intstr.FromInt32(1)},
		{Key: "max-size", Value: intstr.FromInt32(3)},
	}
}

func TestGenerateZoneObjectsOracle(t *testing.T) {
	zone := oracleZone("team-a", flexRequirements()...)
	objs, err := GenerateZoneObjects(zone, oracleRequired("tools", "main"), nil, logging.NewNopLogger())
	if err != nil {
		t.Fatalf("GenerateZoneObjects: %v", err)
	}
	for key := range objs {
		if strings.HasPrefix(key, "launchtemplate-") || strings.HasPrefix(key, "role-") ||
			strings.HasPrefix(key, "rpa-") || strings.HasPrefix(key, "ae-") {
			t.Errorf("unexpected AWS object %s", key)
		}
	}
	np, ok := objs[GetOracleNodePoolKey("default")].(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("node pool not generated, got keys %v", keys(objs))
	}
	want := map[string]any{
		"name":              "team-a-default",
		"clusterId":         "ocid1.cluster.x",
		"compartmentId":     "ocid1.compartment.x",
		"kubernetesVersion": "v1.34.1",
		"nodeShape":         "VM.Standard.E5.Flex",
		"nodeShapeConfig":   []any{map[string]any{"ocpus": float64(2), "memoryInGbs": float64(16)}},
		"nodeSourceDetails": []any{map[string]any{
			"sourceType": "IMAGE", "imageId": "ocid1.image.tools", "bootVolumeSizeInGbs": "50",
		}},
		"nodeConfigDetails": []any{map[string]any{
			"nsgIds":   []any{"ocid1.nsg.node"},
			"kmsKeyId": "ocid1.key.data",
			"placementConfigs": []any{
				map[string]any{"availabilityDomain": "AD-1", "subnetId": "ocid1.subnet.private0"},
				map[string]any{"availabilityDomain": "AD-2", "subnetId": "ocid1.subnet.private1"},
			},
			"nodePoolPodNetworkOptionDetails": []any{map[string]any{
				"cniType": "OCI_VCN_IP_NATIVE", "maxPodsPerNode": float64(31),
				"podSubnetIds": []any{"ocid1.subnet.pod0"}, "podNsgIds": []any{"ocid1.nsg.pod"},
			}},
		}},
		"initialNodeLabels": []any{
			map[string]any{"key": base.TenancyZoneLabel, "value": "team-a"},
			map[string]any{"key": zonePoolAnnotation, "value": "team-a-default"},
		},
		"freeformTags": map[string]any{
			"env":                          "test",
			"tenancy-entigo-com-zone":      "team-a",
			"tenancy-entigo-com-zone-pool": "team-a-default",
			"cluster-autoscaler":           "enabled",
			"minSize":                      "1",
			"maxSize":                      "3",
		},
	}
	got, _, _ := unstructured.NestedMap(np.Object, "spec", "forProvider")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("forProvider mismatch (-want +got):\n%s", diff)
	}
	size, _, _ := unstructured.NestedSlice(np.Object, "spec", "initProvider", "nodeConfigDetails")
	if diff := cmp.Diff([]any{map[string]any{"size": float64(1)}}, size); diff != "" {
		t.Errorf("initProvider mismatch (-want +got):\n%s", diff)
	}
	provider, _, _ := unstructured.NestedString(np.Object, "spec", "providerConfigRef", "name")
	if provider != "crossplane-oracle" {
		t.Errorf("providerConfigRef = %q, want crossplane-oracle", provider)
	}
}

func TestGetOracleNodePool(t *testing.T) {
	tests := map[string]struct {
		zone       string
		reference  []string
		extra      []v1alpha1.Requirement
		override   []v1alpha1.Requirement
		wantErr    string
		wantNil    bool
		wantName   string
		wantImage  string
		wantPlaced []string
		wantSpot   bool
	}{
		"falls back to the next reference pool": {
			reference: []string{"main"}, wantName: "team-a-default", wantImage: "ocid1.image.main",
			wantPlaced: []string{"AD-1", "AD-2"},
		},
		"zone filters availability domains": {
			reference: []string{"tools"}, extra: []v1alpha1.Requirement{{Key: "zone", Values: []string{"AD-2"}}},
			wantName: "team-a-default", wantImage: "ocid1.image.tools", wantPlaced: []string{"AD-2"},
		},
		"image override and spot": {
			reference: []string{"tools"},
			extra: []v1alpha1.Requirement{
				{Key: requirementImage, Value: intstr.FromString("ocid1.image.arm")},
				{Key: "capacity-type", Value: intstr.FromString("SPOT")},
			},
			wantName: "team-a-default", wantImage: "ocid1.image.arm", wantPlaced: []string{"AD-1", "AD-2"}, wantSpot: true,
		},
		"long names are shortened for OCI": {
			zone: "a-very-long-zone-name-for-testing", reference: []string{"tools"},
			wantName:  "a-very-long-zone-name-f-*",
			wantImage: "ocid1.image.tools", wantPlaced: []string{"AD-1", "AD-2"},
		},
		"zero max size skips the pool": {
			reference: []string{"tools"},
			override: []v1alpha1.Requirement{
				{Key: "instance-type", Values: []string{"VM.Standard.E5.Flex"}},
				{Key: requirementOCPUs, Value: intstr.FromInt32(2)},
				{Key: "min-size", Value: intstr.FromInt32(0)},
			},
			wantNil: true,
		},
		"flex shape without ocpus": {
			reference: []string{"tools"},
			override:  []v1alpha1.Requirement{{Key: "instance-type", Values: []string{"VM.Standard.E5.Flex"}}},
			wantErr:   "requires oci/ocpus",
		},
		"several instance types": {
			reference: []string{"tools"},
			override:  []v1alpha1.Requirement{{Key: "instance-type", Values: []string{"VM.Standard3.Flex", "VM.Standard.E5.Flex"}}},
			wantErr:   "exactly one instance-type",
		},
		"no reference pool": {
			reference: nil,
			wantErr:   "none of the reference node pools",
		},
		"zone matches no availability domain": {
			reference: []string{"tools"}, extra: []v1alpha1.Requirement{{Key: "zone", Values: []string{"AD-9"}}},
			wantErr: "no availability domain",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			zoneName := tc.zone
			if zoneName == "" {
				zoneName = "team-a"
			}
			requirements := append(flexRequirements(), tc.extra...)
			if tc.override != nil {
				requirements = tc.override
			}
			objs, err := GenerateZoneObjects(oracleZone(zoneName, requirements...),
				oracleRequired(tc.reference...), nil, logging.NewNopLogger())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GenerateZoneObjects: %v", err)
			}
			np, ok := objs[GetOracleNodePoolKey("default")].(*unstructured.Unstructured)
			if tc.wantNil {
				if ok {
					t.Fatalf("node pool generated, want none")
				}
				return
			}
			if !ok {
				t.Fatalf("node pool not generated")
			}
			fp, _, _ := unstructured.NestedMap(np.Object, "spec", "forProvider")
			got, _ := fp["name"].(string)
			if prefix, ok := strings.CutSuffix(tc.wantName, "*"); ok {
				if !strings.HasPrefix(got, prefix) || len(got) != ociNodePoolNameMaxLength {
					t.Errorf("name = %s, want %s followed by a hash, %d characters", got, prefix, ociNodePoolNameMaxLength)
				}
			} else if got != tc.wantName {
				t.Errorf("name = %s, want %s", got, tc.wantName)
			}
			if got := nestedSliceString(fp, "imageId", "nodeSourceDetails"); got != tc.wantImage {
				t.Errorf("image = %s, want %s", got, tc.wantImage)
			}
			placements, _ := nestedSliceMap(fp, "nodeConfigDetails")["placementConfigs"].([]any)
			var domains []string
			for _, p := range placements {
				placement := p.(map[string]any)
				domains = append(domains, placement["availabilityDomain"].(string))
				if _, spot := placement["preemptibleNodeConfig"]; spot != tc.wantSpot {
					t.Errorf("preemptible = %v, want %v", spot, tc.wantSpot)
				}
			}
			if diff := cmp.Diff(tc.wantPlaced, domains); diff != "" {
				t.Errorf("availability domains (-want +got):\n%s", diff)
			}
		})
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
