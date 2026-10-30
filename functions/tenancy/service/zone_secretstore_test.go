package service

import (
	"encoding/json"
	"testing"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	esmeta "github.com/external-secrets/external-secrets/apis/meta/v1"
	"github.com/google/go-cmp/cmp"
	eksv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/eks/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/iam/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testClusterArn = "arn:aws:eks:eu-north-1:111111111111:cluster/test-cluster"
	testOIDCIssuer = "https://oidc.eks.eu-north-1.amazonaws.com/id/ABC123"
	testOIDC       = "oidc.eks.eu-north-1.amazonaws.com/id/ABC123"
)

var testSecretStoreEnv = apis.Environment{AWSProvider: "aws", SecretStoreNamespace: "external-secrets"}

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
		t.Fatalf("expected no objects without secretStoreNamespace, got %d", len(objs))
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

func TestGenerateSecretStore(t *testing.T) {
	g := newSecretStoreTestGenerator(testSecretStoreEnv, base.StringPtr(testClusterArn), base.StringPtr(testOIDCIssuer))
	objs, err := g.generateSecretStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objs) != 4 {
		t.Fatalf("expected 4 objects, got %d", len(objs))
	}

	role := objs[GetSecretStoreRoleKey("teama")].(*iamv1beta1.Role)
	if role.Name != "teama-secrets" {
		t.Errorf("role name = %s", role.Name)
	}
	wantTrust := []any{map[string]any{
		"Effect":    "Allow",
		"Principal": map[string]any{"Federated": "arn:aws:iam::111111111111:oidc-provider/" + testOIDC},
		"Action":    "sts:AssumeRoleWithWebIdentity",
		"Condition": map[string]any{"StringEquals": map[string]any{
			testOIDC + ":aud": "sts.amazonaws.com",
			testOIDC + ":sub": "system:serviceaccount:external-secrets:zone-teama",
		}},
	}}
	if diff := cmp.Diff(wantTrust, decodeStatements(t, *role.Spec.ForProvider.AssumeRolePolicy)); diff != "" {
		t.Errorf("trust policy mismatch (-want +got):\n%s", diff)
	}

	rolePolicy := objs[GetSecretStoreRolePolicyKey("teama")].(*iamv1beta1.RolePolicy)
	if rolePolicy.Spec.ForProvider.RoleRef.Name != "teama-secrets" {
		t.Errorf("role policy roleRef = %s", rolePolicy.Spec.ForProvider.RoleRef.Name)
	}
	wantPermissions := []any{map[string]any{
		"Sid":    "ReadZoneSecrets",
		"Effect": "Allow",
		"Action": []any{
			"secretsmanager:GetSecretValue",
			"secretsmanager:DescribeSecret",
			"secretsmanager:ListSecretVersionIds",
			"secretsmanager:GetResourcePolicy",
		},
		"Resource":  "arn:aws:secretsmanager:eu-north-1:111111111111:secret:*",
		"Condition": map[string]any{"StringEquals": map[string]any{"aws:ResourceTag/entigo:zone": "teama"}},
	}}
	if diff := cmp.Diff(wantPermissions, decodeStatements(t, *rolePolicy.Spec.ForProvider.Policy)); diff != "" {
		t.Errorf("permission policy mismatch (-want +got):\n%s", diff)
	}

	sa := objs[GetSecretStoreServiceAccountKey("teama")].(*corev1.ServiceAccount)
	if sa.Name != "zone-teama" || sa.Namespace != "external-secrets" {
		t.Errorf("service account = %s/%s", sa.Namespace, sa.Name)
	}
	if got := sa.Annotations[irsaRoleAnnotation]; got != "arn:aws:iam::111111111111:role/teama-secrets" {
		t.Errorf("service account role annotation = %s", got)
	}

	store := objs[GetSecretStoreKey("teama")].(*esv1.ClusterSecretStore)
	if store.Name != "zone-teama" {
		t.Errorf("store name = %s", store.Name)
	}
	wantSpec := esv1.SecretStoreSpec{
		Conditions: []esv1.ClusterSecretStoreCondition{{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{base.TenancyZoneLabel: "teama"}},
		}},
		Provider: &esv1.SecretStoreProvider{AWS: &esv1.AWSProvider{
			Service: esv1.AWSServiceSecretsManager,
			Region:  "eu-north-1",
			Auth: esv1.AWSAuth{JWTAuth: &esv1.AWSJWTAuth{
				ServiceAccountRef: &esmeta.ServiceAccountSelector{Name: "zone-teama", Namespace: base.StringPtr("external-secrets")},
			}},
		}},
	}
	if diff := cmp.Diff(wantSpec, store.Spec); diff != "" {
		t.Errorf("store spec mismatch (-want +got):\n%s", diff)
	}
}

func decodeStatements(t *testing.T, raw string) []any {
	t.Helper()
	var policy map[string]any
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		t.Fatalf("invalid policy json: %v", err)
	}
	statements, _ := policy["Statement"].([]any)
	return statements
}
