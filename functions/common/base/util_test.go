package base

import (
	"testing"

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
								map[string]interface{}{
									"type":   "Ready",
									"status": "True",
								},
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
								map[string]interface{}{
									"type":   "Ready",
									"status": "False",
								},
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
				t.Errorf("getReadyStatus() = %v, want %v", got, tc.want)
			}
		})
	}
}
