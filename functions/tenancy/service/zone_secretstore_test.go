package service

import (
	"encoding/json"
	"testing"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/google/go-cmp/cmp"
	eksv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/eks/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/iam/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	testClusterArn = "arn:aws:eks:eu-north-1:111111111111:cluster/test-cluster"
	testOIDCIssuer = "https://oidc.eks.eu-north-1.amazonaws.com/id/ABC123"
	testOIDC       = "oidc.eks.eu-north-1.amazonaws.com/id/ABC123"
)

var testSecretStoreEnv = apis.Environment{AWSProvider: "aws", SecretStore: apis.SecretStore{Namespace: "external-secrets"}}

func newSecretStoreTestGenerator(env apis.Environment, clusterArn, issuer *string) awsZoneGenerator {
	var cluster eksv1beta1.Cluster
	cluster.Status.AtProvider.Arn = clusterArn
	if issuer != nil {
		cluster.Status.AtProvider.Identity = []eksv1beta1.IdentityObservation{{
			Oidc: []eksv1beta1.OidcObservation{{Issuer: issuer}},
		}}
	}
	return awsZoneGenerator{
		zoneGenerator: zoneGenerator{
			zone:            v1alpha1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "teama"}},
			env:             env,
			zoneAnnotations: map[string]string{base.TenancyZoneLabel: "teama"},
			zoneTags:        map[string]*string{base.TenancyZoneLabel: base.StringPtr("teama")},
		},
		cluster: cluster,
	}
}

func TestGenerateSecretStoreDisabled(t *testing.T) {
	g := newSecretStoreTestGenerator(apis.Environment{AWSProvider: "aws"}, base.StringPtr(testClusterArn), base.StringPtr(testOIDCIssuer))
	objs, err := g.generateSecretStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objs) != 0 {
		t.Fatalf("expected no objects without secretStore.namespace, got %d", len(objs))
	}
}

func TestGenerateSecretStoreInvalidCluster(t *testing.T) {
	tests := map[string]struct {
		arn, issuer *string
	}{
		"missing arn":    {nil, base.StringPtr(testOIDCIssuer)},
		"malformed arn":  {base.StringPtr("test-cluster"), base.StringPtr(testOIDCIssuer)},
		"arn no region":  {base.StringPtr("arn:aws:eks::111111111111:cluster/test-cluster"), base.StringPtr(testOIDCIssuer)},
		"missing issuer": {base.StringPtr(testClusterArn), nil},
		"empty issuer":   {base.StringPtr(testClusterArn), base.StringPtr("https://")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := newSecretStoreTestGenerator(testSecretStoreEnv, tc.arn, tc.issuer).generateSecretStore(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseArnPartition(t *testing.T) {
	arn, err := parseArn("arn:aws-us-gov:eks:us-gov-west-1:222222222222:cluster/c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := arn.roleArn("r"); got != "arn:aws-us-gov:iam::222222222222:role/r" {
		t.Errorf("roleArn = %s", got)
	}
	if got := arn.oidcProviderArn("p"); got != "arn:aws-us-gov:iam::222222222222:oidc-provider/p" {
		t.Errorf("oidcProviderArn = %s", got)
	}
	if got := arn.secretsArn(); got != "arn:aws-us-gov:secretsmanager:us-gov-west-1:222222222222:secret:*" {
		t.Errorf("secretsArn = %s", got)
	}
}

func TestGenerateSecretStore(t *testing.T) {
	g := newSecretStoreTestGenerator(testSecretStoreEnv, base.StringPtr(testClusterArn), base.StringPtr(testOIDCIssuer))
	objs, err := g.generateSecretStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objs) != 4 {
		t.Fatalf("expected 4 objects, got %d", len(objs))
	}

	// IRSA: only the Zone's ServiceAccount may assume the role
	role := objs[GetSecretStoreRoleKey("teama")].(*iamv1beta1.Role)
	if role.Name != "teama-secrets" {
		t.Errorf("role name = %s", role.Name)
	}
	wantTrust := []iamStatement{{
		Effect:    "Allow",
		Principal: map[string]string{"Federated": "arn:aws:iam::111111111111:oidc-provider/" + testOIDC},
		Action:    []string{"sts:AssumeRoleWithWebIdentity"},
		Condition: map[string]map[string]string{"StringEquals": {
			testOIDC + ":aud": "sts.amazonaws.com",
			testOIDC + ":sub": "system:serviceaccount:external-secrets:zone-teama",
		}},
	}}
	if diff := cmp.Diff(wantTrust, decodePolicy(t, *role.Spec.ForProvider.AssumeRolePolicy).Statement); diff != "" {
		t.Errorf("trust policy mismatch (-want +got):\n%s", diff)
	}

	// Read only, and only secrets tagged with the Zone
	rolePolicy := objs[GetSecretStoreRolePolicyKey("teama")].(*iamv1beta1.RolePolicy)
	if rolePolicy.Spec.ForProvider.RoleRef.Name != "teama-secrets" {
		t.Errorf("role policy roleRef = %s", rolePolicy.Spec.ForProvider.RoleRef.Name)
	}
	wantPermissions := []iamStatement{{
		Sid:    "ReadZoneSecrets",
		Effect: "Allow",
		Action: []string{
			"secretsmanager:GetSecretValue",
			"secretsmanager:DescribeSecret",
			"secretsmanager:ListSecretVersionIds",
			"secretsmanager:GetResourcePolicy",
		},
		Resource:  "arn:aws:secretsmanager:eu-north-1:111111111111:secret:*",
		Condition: map[string]map[string]string{"StringEquals": {"aws:ResourceTag/entigo:zone": "teama"}},
	}}
	if diff := cmp.Diff(wantPermissions, decodePolicy(t, *rolePolicy.Spec.ForProvider.Policy).Statement); diff != "" {
		t.Errorf("permission policy mismatch (-want +got):\n%s", diff)
	}

	sa := objs[GetSecretStoreServiceAccountKey("teama")].(*corev1.ServiceAccount)
	if sa.Name != "zone-teama" || sa.Namespace != "external-secrets" {
		t.Errorf("service account = %s/%s", sa.Namespace, sa.Name)
	}
	if got := sa.Annotations[irsaRoleAnnotation]; got != "arn:aws:iam::111111111111:role/teama-secrets" {
		t.Errorf("service account role annotation = %s", got)
	}

	store := objs[GetSecretStoreKey("teama")].(*unstructured.Unstructured)
	if store.GetKind() != "ClusterSecretStore" || store.GetName() != "zone-teama" {
		t.Errorf("store = %s/%s", store.GetKind(), store.GetName())
	}
	expectString(t, store, "teama", "spec", "conditions", "0", "namespaceSelector", "matchLabels", base.TenancyZoneLabel)
	expectString(t, store, "eu-north-1", "spec", "provider", "aws", "region")
	expectString(t, store, "zone-teama", "spec", "provider", "aws", "auth", "jwt", "serviceAccountRef", "name")
	expectString(t, store, "external-secrets", "spec", "provider", "aws", "auth", "jwt", "serviceAccountRef", "namespace")
	if _, found, _ := unstructured.NestedString(store.Object, "spec", "provider", "aws", "role"); found {
		t.Error("store must authenticate with the Zone's ServiceAccount, not assume a role")
	}
}

func decodePolicy(t *testing.T, raw string) iamPolicy {
	t.Helper()
	var policy iamPolicy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		t.Fatalf("invalid policy json: %v", err)
	}
	return policy
}

// expectString walks maps and, for numeric path segments, slices.
func expectString(t *testing.T, obj *unstructured.Unstructured, want string, path ...string) {
	t.Helper()
	var cur any = obj.Object
	for _, p := range path {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			if p != "0" || len(v) == 0 {
				t.Fatalf("unsupported index %s in %v", p, path)
			}
			cur = v[0]
		default:
			t.Fatalf("path %v not found", path)
		}
	}
	if cur != want {
		t.Errorf("%v = %v, want %s", path, cur, want)
	}
}
