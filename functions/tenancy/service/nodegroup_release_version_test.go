package service

import (
	"testing"

	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	ec2v1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/ec2/v1beta1"
	eksv1beta2 "github.com/upbound/provider-aws/v2/apis/cluster/eks/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func newTestAWSGenerator(env apis.Environment) awsZoneGenerator {
	clusterVersion := "1.34"
	cluster := eksv1beta2.Cluster{}
	cluster.SetName("test-cluster")
	cluster.Status.AtProvider.Version = &clusterVersion

	subnetID := "subnet-a-id"
	subnet := &ec2v1beta1.Subnet{}
	subnet.Status.AtProvider.ID = &subnetID

	return awsZoneGenerator{
		zoneGenerator: zoneGenerator{
			zone: v1alpha1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "test-zone"}},
			env:  env,
		},
		cluster:        cluster,
		vpc:            ec2v1beta1.VPC{},
		computeSubnets: []*ec2v1beta1.Subnet{subnet},
	}
}

func testPool(amiReleaseVersion *string) v1alpha1.Pool {
	reqs := []v1alpha1.Requirement{
		{Key: "instance-type", Values: []string{"t3.large"}},
		{Key: "ami-type", Value: intstr.FromString("AL2023_x86_64_STANDARD")},
		{Key: "min-size", Value: intstr.FromInt(1)},
		{Key: "max-size", Value: intstr.FromInt(2)},
	}
	if amiReleaseVersion != nil {
		reqs = append(reqs, v1alpha1.Requirement{Key: "ami-release-version", Value: intstr.FromString(*amiReleaseVersion)})
	}
	return v1alpha1.Pool{Name: "default", Requirements: reqs}
}

func nodeGroupReleaseVersion(t *testing.T, g awsZoneGenerator, pool v1alpha1.Pool) (string, bool) {
	t.Helper()
	ltVersion := float64(1)
	lt := ec2v1beta1.LaunchTemplate{}
	lt.Status.AtProvider.LatestVersion = &ltVersion

	_, obj, err := g.getNodeGroup(pool, lt)
	if err != nil {
		t.Fatalf("getNodeGroup: %v", err)
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("expected *unstructured.Unstructured, got %T", obj)
	}
	rv, found, err := unstructured.NestedString(u.Object, "spec", "forProvider", "releaseVersion")
	if err != nil {
		t.Fatalf("NestedString: %v", err)
	}
	return rv, found
}

// TestGetNodeGroupReleaseVersion drives the real NodeGroup generation to confirm the
// ami-release-version pool requirement reaches NodeGroup.spec.forProvider.releaseVersion.
func TestGetNodeGroupReleaseVersion(t *testing.T) {
	const pin = "1.36.5-20261119"

	t.Run("no requirement leaves releaseVersion unset", func(t *testing.T) {
		g := newTestAWSGenerator(apis.Environment{AWSProvider: "aws-provider"})
		rv, found := nodeGroupReleaseVersion(t, g, testPool(nil))
		if found {
			t.Fatalf("expected releaseVersion to be unset, got %q", rv)
		}
	})

	t.Run("latest leaves releaseVersion unset", func(t *testing.T) {
		g := newTestAWSGenerator(apis.Environment{AWSProvider: "aws-provider"})
		latest := "latest"
		rv, found := nodeGroupReleaseVersion(t, g, testPool(&latest))
		if found {
			t.Fatalf("expected releaseVersion to be unset for latest, got %q", rv)
		}
	})

	t.Run("explicit pin sets releaseVersion", func(t *testing.T) {
		g := newTestAWSGenerator(apis.Environment{AWSProvider: "aws-provider"})
		value := pin
		rv, found := nodeGroupReleaseVersion(t, g, testPool(&value))
		if !found || rv != pin {
			t.Fatalf("expected releaseVersion %q, got %q (found=%v)", pin, rv, found)
		}
	})

	t.Run("infralib aligns to environment version by ami-type", func(t *testing.T) {
		g := newTestAWSGenerator(apis.Environment{AWSProvider: "aws-provider", AMIReleaseVersion: `{"AL2023_x86_64_STANDARD":"1.36.5-20261119"}`})
		value := "infralib"
		rv, found := nodeGroupReleaseVersion(t, g, testPool(&value))
		if !found || rv != pin {
			t.Fatalf("expected aligned releaseVersion %q, got %q (found=%v)", pin, rv, found)
		}
	})

	t.Run("invalid value errors", func(t *testing.T) {
		g := newTestAWSGenerator(apis.Environment{AWSProvider: "aws-provider"})
		value := "banana"
		ltVersion := float64(1)
		lt := ec2v1beta1.LaunchTemplate{}
		lt.Status.AtProvider.LatestVersion = &ltVersion
		if _, _, err := g.getNodeGroup(testPool(&value), lt); err == nil {
			t.Fatal("expected error for invalid ami-release-version, got none")
		}
	})
}
