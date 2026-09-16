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

	// The agent renders a Terraform list with every element quoted, so a real value arrives looking
	// like '"10.0.16.0/22","10.0.20.0/22"'. Taken verbatim from a rendered biz values file.
	t.Run("quoted cidrs from the agent are unquoted", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular,
			`zone.istioServiceEntries.data.cidrs="10.146.16.0/22"\,"10.146.20.0/22"\,"10.146.0.0/26"`,
			`zone.istioServiceEntries.podCidrs="10.146.32.0/21"\,"10.146.40.0/21"`,
			`zone.istioServiceEntries.service.cidrs="10.146.32.0/21"\,"10.146.40.0/21"`)
		for _, want := range []string{`- "10.146.16.0/22"`, `- "10.146.20.0/22"`, `- "10.146.0.0/26"`} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
		// Without unquoting, the address keeps the agent's quotes and is not a valid CIDR. Scoped
		// to our own values, since unrelated CEL elsewhere in the chart contains escaped quotes.
		if strings.Contains(out, `\"10.146.`) {
			t.Error("an address kept the quotes the agent added")
		}
		// service and podCidrs are the same value here, which is the default subnet_split_mode.
		if strings.Contains(out, "platform-apis-service-subnets") {
			t.Error("the service group rendered despite matching the quoted podCidrs")
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

	// With subnet_split_mode "default" the VPC returns the same private subnet CIDRs for the
	// service and compute subnets, and Pod IPs come from the compute subnets, so the service group
	// would open its ports to every Pod. podCidrs makes that case skip itself.
	t.Run("service is skipped when its cidrs overlap podCidrs", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, dataCIDR,
			`zone.istioServiceEntries.service.cidrs=10.1.0.0/21\,10.1.8.0/21`,
			`zone.istioServiceEntries.podCidrs=10.1.0.0/21\,10.1.8.0/21`)
		if strings.Contains(out, "platform-apis-service-subnets") {
			t.Fatal("the service group rendered even though its cidrs are the Pod cidrs")
		}
		if !strings.Contains(out, "platform-apis-data-subnets") {
			t.Error("the guard wrongly dropped the data group")
		}
	})

	// With "spoke" the two are disjoint, which is when the group is worth having.
	t.Run("service renders when disjoint from podCidrs", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, dataCIDR,
			"zone.istioServiceEntries.service.cidrs=10.1.8.0/21",
			"zone.istioServiceEntries.podCidrs=10.1.16.0/21")
		if !strings.Contains(out, "platform-apis-service-subnets") {
			t.Fatal("the service group was dropped even though it does not overlap the Pod cidrs")
		}
	})

	t.Run("a partial overlap with podCidrs also skips", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, dataCIDR,
			`zone.istioServiceEntries.service.cidrs=10.1.8.0/21\,10.1.16.0/21`,
			"zone.istioServiceEntries.podCidrs=10.1.16.0/21")
		if strings.Contains(out, "platform-apis-service-subnets") {
			t.Fatal("the service group rendered despite overlapping the Pod cidrs")
		}
	})

	// The guard is deliberately not applied to the data group: silently dropping it would break
	// database access rather than over-open it.
	t.Run("the data group ignores podCidrs", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular,
			"zone.istioServiceEntries.data.cidrs=10.1.0.0/21",
			"zone.istioServiceEntries.podCidrs=10.1.0.0/21")
		if !strings.Contains(out, "platform-apis-data-subnets") {
			t.Fatal("the data group was dropped by the podCidrs guard")
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

// TestZoneAWSAPIServiceEntries covers the AWS service entries in
// helm/templates/zone-serviceentries.yaml. They open the AWS APIs a tenant application calls,
// which under granularEgress are otherwise closed - including STS, without which IRSA cannot
// work at all.
func TestZoneAWSAPIServiceEntries(t *testing.T) {
	t.Parallel()
	const (
		granular = "zone.environmentConfig.granularEgress=true"
		region   = "zone.istioServiceEntries.region=eu-north-1"
	)

	t.Run("not rendered while granularEgress is off", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, region)
		if strings.Contains(out, "platform-apis-aws-") {
			t.Fatal("an AWS ServiceEntry was rendered with granularEgress disabled")
		}
	})

	// Every default host is regional, so without a region there is nothing to render. Skipping
	// beats rendering a host with the placeholder still in it, which would silently match nothing.
	t.Run("the group is skipped when no region is set", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular)
		if strings.Contains(out, "platform-apis-aws-") {
			t.Fatal("an AWS ServiceEntry was rendered without a region")
		}
	})

	t.Run("each service renders its own entry", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region)
		for _, want := range []string{
			"name: platform-apis-aws-sts",
			"name: platform-apis-aws-s3",
			"name: platform-apis-aws-ecr",
			"name: platform-apis-aws-rds",
			"name: platform-apis-aws-elasticache",
			"name: platform-apis-aws-kms",
			"name: platform-apis-aws-secretsmanager",
			"name: platform-apis-aws-ec2",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	t.Run("hosts carry the region, the protocol and the resolution Envoy needs", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region)
		for _, want := range []string{
			`- "sts.eu-north-1.amazonaws.com"`,
			`- "kms.eu-north-1.amazonaws.com"`,
			"resolution: DNS",
			"number: 443",
			"protocol: TLS",
			`- "*"`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	// A host that AWS does not regionalise is used as written. Older SDKs still call global STS,
	// and losing it would break them while the regional host kept working.
	t.Run("a host without the placeholder is kept as written", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region)
		if !strings.Contains(out, `- "sts.amazonaws.com"`) {
			t.Error("the global STS host was dropped")
		}
	})

	// Istio accepts a wildcard only as the leading label, so the virtual-hosted bucket and registry
	// forms have to survive substitution intact. sts.*.amazonaws.com would be rejected instead.
	t.Run("wildcard hosts keep their leading label", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region)
		for _, want := range []string{
			`- "*.s3.eu-north-1.amazonaws.com"`,
			`- "*.dkr.ecr.eu-north-1.amazonaws.com"`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	// A leftover placeholder is not a rendering error, it is a host that matches nothing, so the
	// Pod gets a blackhole instead of the API. Scoped to our own hosts.
	t.Run("no host keeps an unsubstituted placeholder", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region)
		if strings.Contains(out, "{region}") {
			t.Error("a host kept the {region} placeholder")
		}
	})

	// Overriding services replaces the default list rather than merging into it, which is how Helm
	// treats every list, so each of these passes a complete list.
	t.Run("a service with no hosts is skipped", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region,
			"zone.istioServiceEntries.awsApis.services[0].name=empty",
			"zone.istioServiceEntries.awsApis.services[1].name=present",
			"zone.istioServiceEntries.awsApis.services[1].hosts[0]=present.{region}.amazonaws.com")
		if strings.Contains(out, "platform-apis-aws-empty") {
			t.Fatal("a service with no hosts rendered a ServiceEntry, which opens nothing")
		}
		if !strings.Contains(out, "platform-apis-aws-present") {
			t.Error("skipping the empty service dropped the one next to it")
		}
	})

	t.Run("a custom service brings its own name, hosts and ports", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region,
			"zone.istioServiceEntries.awsApis.services[0].name=renamed",
			"zone.istioServiceEntries.awsApis.services[0].hosts[0]=example.{region}.amazonaws.com",
			"zone.istioServiceEntries.awsApis.services[0].ports[0].number=8443",
			"zone.istioServiceEntries.awsApis.services[0].ports[0].name=tls-alt",
			"zone.istioServiceEntries.awsApis.services[0].ports[0].protocol=TLS")
		for _, want := range []string{
			"name: platform-apis-aws-renamed",
			`- "example.eu-north-1.amazonaws.com"`,
			"number: 8443",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered custom service is missing %q", want)
			}
		}
		if strings.Contains(out, "number: 443") {
			t.Error("the per-service ports were ignored in favour of the group default")
		}
	})

	t.Run("istioNamespace applies to the AWS entries too", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region, "zone.istioServiceEntries.istioNamespace=mesh")
		if strings.Contains(out, "namespace: istio-system") {
			t.Error("an AWS entry stayed in istio-system after the override")
		}
	})
}

// TestZoneExtraServices covers istioServiceEntries.extraServices, the hostname entries an
// environment adds. Helm replaces a list rather than merging into it, so opening one more endpoint
// through the AWS defaults would mean restating every host to keep them; these are added on top.
func TestZoneExtraServices(t *testing.T) {
	t.Parallel()
	const (
		granular  = "zone.environmentConfig.granularEgress=true"
		region    = "zone.istioServiceEntries.region=eu-north-1"
		extraName = "zone.istioServiceEntries.extraServices[0].name=bedrock"
		extraHost = "zone.istioServiceEntries.extraServices[0].hosts[0]=bedrock-runtime.{region}.amazonaws.com"
	)

	t.Run("an extra service renders next to the defaults", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region, extraName, extraHost)
		for _, want := range []string{
			"name: platform-apis-bedrock-hosts",
			`- "bedrock-runtime.eu-north-1.amazonaws.com"`,
			"name: platform-apis-aws-sts",
			"name: platform-apis-aws-s3",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	t.Run("an extra service can set its own ports", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region,
			"zone.istioServiceEntries.extraServices[0].name=partner",
			"zone.istioServiceEntries.extraServices[0].hosts[0]=api.partner.example.com",
			"zone.istioServiceEntries.extraServices[0].ports[0].number=8443",
			"zone.istioServiceEntries.extraServices[0].ports[0].name=tls-https-alt",
			"zone.istioServiceEntries.extraServices[0].ports[0].protocol=TLS")
		for _, want := range []string{"name: platform-apis-partner-hosts", "number: 8443"} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})

	// An endpoint listed here may not be an AWS one, so it must not be held back by the region the
	// default set needs.
	t.Run("an extra service without a region still renders", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular,
			"zone.istioServiceEntries.extraServices[0].name=partner",
			"zone.istioServiceEntries.extraServices[0].hosts[0]=api.partner.example.com")
		if !strings.Contains(out, "name: platform-apis-partner-hosts") {
			t.Error("an extra service with no regional host was dropped with the defaults")
		}
		if strings.Contains(out, "platform-apis-aws-sts") {
			t.Error("the default services rendered without a region")
		}
	})

	// Rendering sts..amazonaws.com, or the placeholder verbatim, would be a host that quietly
	// matches nothing. Dropping it leaves the entry with only hosts that can work.
	t.Run("a host needing a region is dropped while none is set", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, extraName, extraHost,
			"zone.istioServiceEntries.extraServices[0].hosts[1]=bedrock.example.com")
		if strings.Contains(out, "{region}") || strings.Contains(out, "bedrock-runtime..amazonaws.com") {
			t.Error("a host that needs a region was rendered without one")
		}
		if !strings.Contains(out, `- "bedrock.example.com"`) {
			t.Error("the host that does not need a region was dropped too")
		}
	})

	t.Run("an extra service left with no hosts is skipped", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, extraName, extraHost)
		if strings.Contains(out, "platform-apis-bedrock-hosts") {
			t.Fatal("an extra service whose only host needs a region rendered anyway")
		}
	})

	// Two entries of the same name are two objects with one name in one namespace, where only the
	// last applied survives. Failing the render says so instead.
	t.Run("a name used twice fails the render", func(t *testing.T) {
		t.Parallel()
		out, err := exec.Command("helm", "template", "test-release", chartDir,
			"--set", granular, "--set", region,
			"--set", "zone.istioServiceEntries.extraServices[0].name=partner",
			"--set", "zone.istioServiceEntries.extraServices[0].hosts[0]=api.partner.example.com",
			"--set", "zone.istioServiceEntries.extraServices[1].name=partner",
			"--set", "zone.istioServiceEntries.extraServices[1].hosts[0]=files.partner.example.com").CombinedOutput()
		if err == nil {
			t.Fatal("a duplicate name rendered instead of failing")
		}
		if !strings.Contains(string(out), `"platform-apis-partner-hosts" would be rendered more than once`) {
			t.Errorf("the failure does not name the duplicate: %s", out)
		}
	})

	// An extra named after an AWS service is not a collision: the two render under different
	// names, so both survive.
	t.Run("an extra may reuse an AWS service name", func(t *testing.T) {
		t.Parallel()
		out := renderChart(t, granular, region,
			"zone.istioServiceEntries.extraServices[0].name=s3",
			"zone.istioServiceEntries.extraServices[0].hosts[0]=s3.example.com")
		for _, want := range []string{"name: platform-apis-s3-hosts", "name: platform-apis-aws-s3"} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered output is missing %q", want)
			}
		}
	})
}
