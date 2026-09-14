package test

import (
	"os/exec"
	"strings"
	"testing"
)

// renderChart runs helm template with the given --set overrides and returns the manifest.
func renderChart(t *testing.T, values ...string) string {
	t.Helper()
	args := []string{"template", "test-release", chartDir}
	for _, v := range values {
		args = append(args, "--set", v)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	return string(out)
}

// TestZoneServiceEntries covers helm/templates/zone-serviceentries.yaml, the cluster-wide
// ServiceEntries that let every Zone reach the shared infrastructure subnets while granularEgress
// is on.
func TestZoneServiceEntries(t *testing.T) {
	t.Parallel()
	const (
		granular = "zone.environmentConfig.granularEgress=true"
		dataCIDR = "zone.istioServiceEntries.data.cidrs[0]=10.0.16.0/22"
		svcCIDR  = "zone.istioServiceEntries.service.cidrs[0]=10.0.32.0/21"
	)

	t.Run("not rendered while granularEgress is off", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, dataCIDR, svcCIDR)
		if strings.Contains(out, "kind: ServiceEntry") {
			t.Fatal("a ServiceEntry was rendered with granularEgress disabled")
		}
	})

	// A ServiceEntry with spec.addresses unset matches every destination on its ports instead of
	// none, so an empty cidrs list must skip the object rather than render it without addresses.
	t.Run("a group with no cidrs is skipped", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular)
		if strings.Contains(out, "kind: ServiceEntry") {
			t.Fatal("a ServiceEntry was rendered with no cidrs, which would open every destination")
		}
	})

	t.Run("data group renders with its cidrs and ports", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, dataCIDR)
		for _, want := range []string{
			"name: platform-apis-data-subnets",
			"namespace: istio-system",
			`- "10.0.16.0/22"`,
			"resolution: NONE",
			`- "*"`,
			"number: 5432",
			"number: 6379",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered data ServiceEntry is missing %q", want)
			}
		}
		if strings.Contains(out, "platform-apis-service-subnets") {
			t.Error("the service group rendered even though it has no cidrs")
		}
	})

	t.Run("service group renders separately", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, dataCIDR, svcCIDR)
		for _, want := range []string{"platform-apis-data-subnets", "platform-apis-service-subnets", "number: 8443"} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	t.Run("istioNamespace is configurable", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, dataCIDR, "zone.istioServiceEntries.istioNamespace=mesh")
		if !strings.Contains(out, "namespace: mesh") {
			t.Error("istioNamespace override was not applied")
		}
	})

	t.Run("extraSubnets render their own entry", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular,
			"zone.istioServiceEntries.extraSubnets[0].name=partner",
			"zone.istioServiceEntries.extraSubnets[0].cidrs[0]=10.90.0.0/24",
			"zone.istioServiceEntries.extraSubnets[0].ports[0].number=5432",
			"zone.istioServiceEntries.extraSubnets[0].ports[0].name=tcp-postgres",
			"zone.istioServiceEntries.extraSubnets[0].ports[0].protocol=TCP")
		for _, want := range []string{"platform-apis-partner-subnets", `- "10.90.0.0/24"`, "number: 5432"} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered extraSubnets entry is missing %q", want)
			}
		}
	})

	// infralib passes a subnet output through as one comma separated value, and joins two of them
	// with a comma, so the string form has to work as well as a YAML list.
	t.Run("cidrs accept a comma separated string", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular,
			`zone.istioServiceEntries.data.cidrs=10.0.16.0/22\,10.0.20.0/22\,10.0.0.0/26`)
		for _, want := range []string{`- "10.0.16.0/22"`, `- "10.0.20.0/22"`, `- "10.0.0.0/26"`} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	// An environment with no elasticache subnets yields a trailing comma once the two outputs are
	// joined. The empty segment must be dropped, not rendered as an address.
	t.Run("empty segments in the string are dropped", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, `zone.istioServiceEntries.data.cidrs=10.0.16.0/22\,`)
		if !strings.Contains(out, `- "10.0.16.0/22"`) {
			t.Error("the populated cidr was dropped")
		}
		if strings.Contains(out, `- ""`) {
			t.Error("an empty segment was rendered as an address")
		}
	})

	// Both outputs empty joins to a bare ",". That must skip the group, not render an entry with
	// no addresses, which would match every destination on its ports.
	t.Run("a string of only separators is skipped", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, `zone.istioServiceEntries.data.cidrs=\,`)
		if strings.Contains(out, "kind: ServiceEntry") {
			t.Fatal("a ServiceEntry rendered from an empty cidr string, which would open every destination")
		}
	})

	// Ports have to be listed one by one: ServiceEntry has no port range syntax, and omitting
	// ports opens nothing rather than everything.
	t.Run("an extraSubnets entry without ports is skipped", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular,
			"zone.istioServiceEntries.extraSubnets[0].name=partner",
			"zone.istioServiceEntries.extraSubnets[0].cidrs[0]=10.90.0.0/24")
		if strings.Contains(out, "platform-apis-partner-subnets") {
			t.Fatal("an extraSubnets entry with no ports rendered a ServiceEntry that opens nothing")
		}
	})
}
