package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/entigolabs/function-base/base"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/iam/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretStoreRoleSuffix = "-secrets"
	irsaRoleAnnotation    = "eks.amazonaws.com/role-arn"
)

// The secret store keys are deliberately outside the Zone sequence. A sequence step that is not
// Ready holds back new objects in later steps, so a failing secrets role would stop RBAC and
// node pools for new Namespaces and pools. These objects need no ordering, Crossplane resolves
// the roleRef and the store becomes Ready once the role can be assumed.

func GetSecretStoreRoleKey(zone string) string {
	return "secretstore-role-" + zone
}

func GetSecretStoreRolePolicyKey(zone string) string {
	return "secretstore-policy-" + zone
}

func GetSecretStoreServiceAccountKey(zone string) string {
	return "secretstore-sa-" + zone
}

func GetSecretStoreKey(zone string) string {
	return "secretstore-" + zone
}

// GetSecretStoreName is the ClusterSecretStore that ExternalSecrets in the Zone's Namespaces
// reference. Its ServiceAccount, in the external-secrets Namespace, has the same name.
func GetSecretStoreName(zone string) string {
	return "zone-" + zone
}

// generateSecretStore gives the Zone its own ServiceAccount, IRSA role and ClusterSecretStore.
// The store can only be used from the Zone's Namespaces and only reads AWS Secrets Manager
// secrets tagged entigo:zone=<zone>. The cluster-wide store from infralib is left untouched, so
// ExternalSecrets that reference it keep working.
func (g awsZoneGenerator) generateSecretStore() (map[string]client.Object, error) {
	namespace := g.env.SecretStore.Namespace
	if namespace == "" {
		return nil, nil
	}
	// The secrets live in the cluster's account, so it is taken from the cluster ARN
	clusterArn := g.cluster.Status.AtProvider.Arn
	if clusterArn == nil {
		return nil, errors.New("cluster status has no arn, required for the zone secret store")
	}
	arn, err := parseArn(*clusterArn)
	if err != nil {
		return nil, err
	}
	oidcProvider, err := g.clusterOIDCProvider()
	if err != nil {
		return nil, err
	}
	name := GetSecretStoreName(g.zone.Name)
	roleName := g.zone.Name + secretStoreRoleSuffix
	trustPolicy, err := secretStoreTrustPolicy(arn, oidcProvider, namespace, name)
	if err != nil {
		return nil, err
	}
	permissionPolicy, err := secretStorePermissionPolicy(g.zone.Name, arn.secretsArn())
	if err != nil {
		return nil, err
	}
	return map[string]client.Object{
		GetSecretStoreRoleKey(g.zone.Name):           g.getSecretStoreRole(roleName, trustPolicy),
		GetSecretStoreRolePolicyKey(g.zone.Name):     g.getSecretStoreRolePolicy(roleName, permissionPolicy),
		GetSecretStoreServiceAccountKey(g.zone.Name): g.getSecretStoreServiceAccount(name, namespace, arn.roleArn(roleName)),
		GetSecretStoreKey(g.zone.Name):               g.getSecretStore(name, namespace, arn.region),
	}, nil
}

// clusterOIDCProvider returns the cluster's IAM OIDC provider, the issuer URL without its scheme.
func (g awsZoneGenerator) clusterOIDCProvider() (string, error) {
	identity := g.cluster.Status.AtProvider.Identity
	if len(identity) == 0 || len(identity[0].Oidc) == 0 || identity[0].Oidc[0].Issuer == nil {
		return "", errors.New("cluster status has no oidc issuer, required for the zone secret store")
	}
	provider := strings.TrimPrefix(*identity[0].Oidc[0].Issuer, "https://")
	if provider == "" {
		return "", errors.New("cluster oidc issuer is empty")
	}
	return provider, nil
}

type awsArn struct {
	partition, region, account string
}

func parseArn(value string) (awsArn, error) {
	parts := strings.SplitN(value, ":", 6)
	if len(parts) != 6 || parts[0] != "arn" || parts[1] == "" || parts[3] == "" || parts[4] == "" {
		return awsArn{}, fmt.Errorf("cannot read partition, region and account from arn %q", value)
	}
	return awsArn{partition: parts[1], region: parts[3], account: parts[4]}, nil
}

func (a awsArn) roleArn(name string) string {
	return fmt.Sprintf("arn:%s:iam::%s:role/%s", a.partition, a.account, name)
}

func (a awsArn) oidcProviderArn(provider string) string {
	return fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", a.partition, a.account, provider)
}

func (a awsArn) secretsArn() string {
	return fmt.Sprintf("arn:%s:secretsmanager:%s:%s:secret:*", a.partition, a.region, a.account)
}

func (g awsZoneGenerator) getSecretStoreRole(name, trustPolicy string) *iamv1beta1.Role {
	return &iamv1beta1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "iam.aws.upbound.io/v1beta1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: g.zoneAnnotations,
		},
		Spec: iamv1beta1.RoleSpec{
			ResourceSpec: v1.ResourceSpec{
				ManagementPolicies: []v1.ManagementAction{v1.ManagementActionAll},
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
			ForProvider: iamv1beta1.RoleParameters{
				AssumeRolePolicy: base.StringPtr(trustPolicy),
				Tags:             g.zoneTags,
			},
		},
	}
}

func (g awsZoneGenerator) getSecretStoreRolePolicy(roleName, policy string) *iamv1beta1.RolePolicy {
	return &iamv1beta1.RolePolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "iam.aws.upbound.io/v1beta1",
			Kind:       "RolePolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        roleName,
			Annotations: g.zoneAnnotations,
		},
		Spec: iamv1beta1.RolePolicySpec{
			ResourceSpec: v1.ResourceSpec{
				ManagementPolicies: []v1.ManagementAction{v1.ManagementActionAll},
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
			ForProvider: iamv1beta1.RolePolicyParameters{
				Policy:  base.StringPtr(policy),
				RoleRef: &v1.Reference{Name: roleName},
			},
		},
	}
}

// getSecretStoreServiceAccount is the identity the store authenticates as. It lives in the
// external-secrets Namespace, out of reach of the Zone's users, and IRSA maps it to the Zone role.
func (g awsZoneGenerator) getSecretStoreServiceAccount(name, namespace, roleArn string) *corev1.ServiceAccount {
	annotations := map[string]string{irsaRoleAnnotation: roleArn}
	for k, v := range g.zoneAnnotations {
		annotations[k] = v
	}
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

func (g awsZoneGenerator) getSecretStore(name, namespace, region string) client.Object {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "external-secrets.io/v1",
		"kind":       "ClusterSecretStore",
		"metadata": map[string]any{
			"name":        name,
			"annotations": toAnyMap(g.zoneAnnotations),
			"labels":      map[string]any{base.TenancyZoneLabel: g.zone.Name},
		},
		"spec": map[string]any{
			// Only the Zone's own Namespaces may reference this store
			"conditions": []any{map[string]any{
				"namespaceSelector": map[string]any{
					"matchLabels": map[string]any{base.TenancyZoneLabel: g.zone.Name},
				},
			}},
			"provider": map[string]any{
				"aws": map[string]any{
					"service": "SecretsManager",
					"region":  region,
					"auth": map[string]any{
						"jwt": map[string]any{
							"serviceAccountRef": map[string]any{
								"name":      name,
								"namespace": namespace,
							},
						},
					},
				},
			},
		},
	}}
}

func toAnyMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

type iamPolicy struct {
	Version   string         `json:"Version"`
	Statement []iamStatement `json:"Statement"`
}

type iamStatement struct {
	Sid       string                       `json:"Sid,omitempty"`
	Effect    string                       `json:"Effect"`
	Principal map[string]string            `json:"Principal,omitempty"`
	Action    []string                     `json:"Action"`
	Resource  string                       `json:"Resource,omitempty"`
	Condition map[string]map[string]string `json:"Condition,omitempty"`
}

func marshalPolicy(policy iamPolicy) (string, error) {
	out, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// secretStoreTrustPolicy lets only the Zone's ServiceAccount assume the Zone role through IRSA,
// the same way infralib's external-secrets role trusts its own ServiceAccount.
func secretStoreTrustPolicy(arn awsArn, oidcProvider, namespace, serviceAccount string) (string, error) {
	return marshalPolicy(iamPolicy{
		Version: "2012-10-17",
		Statement: []iamStatement{{
			Effect:    "Allow",
			Principal: map[string]string{"Federated": arn.oidcProviderArn(oidcProvider)},
			Action:    []string{"sts:AssumeRoleWithWebIdentity"},
			Condition: map[string]map[string]string{"StringEquals": {
				oidcProvider + ":aud": "sts.amazonaws.com",
				oidcProvider + ":sub": fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccount),
			}},
		}},
	})
}

// secretStorePermissionPolicy lets the Zone role read only secrets tagged with the Zone's name.
// The tag is the platform's AWS tag key, entigo:zone, not the Kubernetes label key: a "/" in the
// key would be read as the separator in an aws:ResourceTag/<key> condition.
func secretStorePermissionPolicy(zone, secrets string) (string, error) {
	return marshalPolicy(iamPolicy{
		Version: "2012-10-17",
		Statement: []iamStatement{{
			Sid:    "ReadZoneSecrets",
			Effect: "Allow",
			Action: []string{
				"secretsmanager:GetSecretValue",
				"secretsmanager:DescribeSecret",
				"secretsmanager:ListSecretVersionIds",
				"secretsmanager:GetResourcePolicy",
			},
			Resource:  secrets,
			Condition: map[string]map[string]string{"StringEquals": {"aws:ResourceTag/" + base.TenancyZoneAWSTag: zone}},
		}},
	})
}
