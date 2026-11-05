package service

import (
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResolveSecretStoreName(t *testing.T) {
	env := apis.Environment{EsClusterSecretStore: "external-secrets"}

	required := func(present bool, zone string) map[string][]resource.Required {
		if !present {
			return map[string][]resource.Required{}
		}
		meta := map[string]any{"name": "testspace"}
		if zone != "" {
			meta["labels"] = map[string]any{base.TenancyZoneLabel: zone}
		}
		ns := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   meta,
		}}
		return map[string][]resource.Required{base.NamespaceKey: {{Resource: ns}}}
	}

	tests := map[string]struct {
		required map[string][]resource.Required
		want     string
	}{
		"a zoned namespace reads through its own store": {required(true, "a"), "zone-a"},
		"a namespace without a zone falls back":          {required(true, ""), "external-secrets"},
		"a missing namespace falls back":                 {required(false, ""), "external-secrets"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resolveSecretStoreName(tc.required, env); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
