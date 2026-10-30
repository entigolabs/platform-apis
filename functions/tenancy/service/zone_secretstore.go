package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/entigolabs/function-base/base"
	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	esmeta "github.com/external-secrets/external-secrets/apis/meta/v1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/iam/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretStoreRoleSuffix = "-secrets"
	irsaRoleAnnotation    = "eks.amazonaws.com/role-arn"
)

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

// GetSecretStoreName is also the name of the store's ServiceAccount.
func GetSecretStoreName(zone string) string {
	return "zone-" + zone
}

// generateSecretStore objects are kept out of the Zone sequence on purpose. A step that is not Ready
// holds back later steps, so a failing secrets role would block RBAC and node pools.
func (g awsZoneGenerator) generateSecretStore() (map[string]client.Object, error) {
	namespace := g.env.SecretStoreNamespace
	if namespace == "" {
		return nil, nil
	}
	region, account, err := g.clusterRegionAndAccount()
	if err != nil {
		return nil, err
	}
	oidcProvider, err := g.clusterOIDCProvider()
	if err != nil {
		return nil, err
	}
	name := GetSecretStoreName(g.zone.Name)
	roleName := g.zone.Name + secretStoreRoleSuffix
	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", account, roleName)
	return map[string]client.Object{
		GetSecretStoreRoleKey(g.zone.Name):           g.getIAMRole(roleName, secretStoreAssumeRolePolicy(account, oidcProvider, namespace, name)),
		GetSecretStoreRolePolicyKey(g.zone.Name):     g.getSecretStoreRolePolicy(roleName, secretStorePermissionPolicy(g.zone.Name, region, account)),
		GetSecretStoreServiceAccountKey(g.zone.Name): g.getSecretStoreServiceAccount(name, namespace, roleArn),
		GetSecretStoreKey(g.zone.Name):               g.getSecretStore(name, namespace, region),
	}, nil
}

// clusterRegionAndAccount reads them from the cluster ARN, as the secrets live in the cluster's account.
func (g awsZoneGenerator) clusterRegionAndAccount() (string, string, error) {
	clusterArn := g.cluster.Status.AtProvider.Arn
	if clusterArn == nil {
		return "", "", errors.New("cluster status has no arn, required for the zone secret store")
	}
	parts := strings.Split(*clusterArn, ":")
	if len(parts) < 6 || parts[3] == "" || parts[4] == "" {
		return "", "", fmt.Errorf("cannot read region and account from cluster arn %q", *clusterArn)
	}
	return parts[3], parts[4], nil
}

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

// getSecretStoreServiceAccount lives in the external-secrets Namespace, out of reach of the Zone's
// users, as anyone able to use it could read the Zone's secrets.
func (g awsZoneGenerator) getSecretStoreServiceAccount(name, namespace, roleArn string) *corev1.ServiceAccount {
	annotations := map[string]string{irsaRoleAnnotation: roleArn}
	maps.Copy(annotations, g.zoneAnnotations)
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

func (g awsZoneGenerator) getSecretStore(name, namespace, region string) *esv1.ClusterSecretStore {
	return &esv1.ClusterSecretStore{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "external-secrets.io/v1",
			Kind:       "ClusterSecretStore",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: g.zoneAnnotations,
			Labels:      map[string]string{base.TenancyZoneLabel: g.zone.Name},
		},
		Spec: esv1.SecretStoreSpec{
			Conditions: []esv1.ClusterSecretStoreCondition{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{base.TenancyZoneLabel: g.zone.Name},
				},
			}},
			Provider: &esv1.SecretStoreProvider{
				AWS: &esv1.AWSProvider{
					Service: esv1.AWSServiceSecretsManager,
					Region:  region,
					Auth: esv1.AWSAuth{
						JWTAuth: &esv1.AWSJWTAuth{
							ServiceAccountRef: &esmeta.ServiceAccountSelector{Name: name, Namespace: &namespace},
						},
					},
				},
			},
		},
	}
}

// secretStoreAssumeRolePolicy mirrors the trust policy of infralib's external-secrets role.
func secretStoreAssumeRolePolicy(account, oidcProvider, namespace, serviceAccount string) string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect": "Allow",
				"Principal": map[string]any{
					"Federated": fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", account, oidcProvider),
				},
				"Action": "sts:AssumeRoleWithWebIdentity",
				"Condition": map[string]any{
					"StringEquals": map[string]any{
						oidcProvider + ":aud": "sts.amazonaws.com",
						oidcProvider + ":sub": fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccount),
					},
				},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

// secretStorePermissionPolicy uses the AWS tag key, not the Kubernetes label key, as a "/" in the
// key would be read as the separator in an aws:ResourceTag/<key> condition.
func secretStorePermissionPolicy(zone, region, account string) string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Sid":    "ReadZoneSecrets",
				"Effect": "Allow",
				"Action": []string{
					"secretsmanager:GetSecretValue",
					"secretsmanager:DescribeSecret",
					"secretsmanager:ListSecretVersionIds",
					"secretsmanager:GetResourcePolicy",
				},
				"Resource": fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:*", region, account),
				"Condition": map[string]any{
					"StringEquals": map[string]any{"aws:ResourceTag/" + base.TenancyZoneAWSTag: zone},
				},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}
