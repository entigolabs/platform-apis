package service

import (
	"fmt"
	"maps"
	"strings"

	mysqlv1alpha1 "github.com/crossplane-contrib/provider-sql/apis/namespaced/mysql/v1alpha1"
	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	xpv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const grantPrefix = "grant-"

type mariaDBUserGenerator struct {
	mariaDBUser        v1alpha1.MariaDBUser
	mariaDBInstance    v1alpha1.MariaDBInstance
	mariaDBDatabase    v1alpha1.MariaDBDatabase
	providerConfigName string
	userDisplayName    string
}

func GenerateMariaDBUserObjects(
	mariaDBUser v1alpha1.MariaDBUser,
	required map[string][]resource.Required,
) (map[string]client.Object, error) {
	g, err := newMariaDBUserGenerator(mariaDBUser, required)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newMariaDBUserGenerator(
	mariaDBUser v1alpha1.MariaDBUser,
	required map[string][]resource.Required,
) (*mariaDBUserGenerator, error) {
	var mariaDBInstance v1alpha1.MariaDBInstance
	if err := base.ExtractRequiredResource(required, "MariaDBInstance", &mariaDBInstance); err != nil {
		return nil, err
	}

	var mariaDBDatabase v1alpha1.MariaDBDatabase
	if mariaDBUser.Spec.DatabaseRef != nil {
		if err := base.ExtractRequiredResource(required, "MariaDBDatabase", &mariaDBDatabase); err != nil {
			return nil, err
		}
	}

	userDisplayName := mariaDBUser.Name
	if mariaDBUser.Spec.Name != "" {
		userDisplayName = mariaDBUser.Spec.Name
	}

	return &mariaDBUserGenerator{
		mariaDBUser:        mariaDBUser,
		mariaDBInstance:    mariaDBInstance,
		mariaDBDatabase:    mariaDBDatabase,
		providerConfigName: mariaDBUser.Spec.InstanceRef.Name + "-providerconfig",
		userDisplayName:    userDisplayName,
	}, nil
}

func (g *mariaDBUserGenerator) generate() (map[string]client.Object, error) {
	desired := make(map[string]client.Object)
	instanceReady := isMariaDBInstanceReady(g.mariaDBInstance)
	if !instanceReady {
		return desired, fmt.Errorf("temporarily waiting for MariaDBInstance %s to become ready", g.mariaDBInstance.Name)
	}

	if g.mariaDBUser.Spec.DatabaseRef != nil && !isMariaDBDatabaseReady(g.mariaDBDatabase) {
		return desired, fmt.Errorf("temporarily waiting for MariaDBDatabase %s to become ready", g.mariaDBUser.Spec.DatabaseRef.Name)
	}

	maps.Copy(desired, g.buildUser())
	maps.Copy(desired, g.buildGrants())
	maps.Copy(desired, g.buildGrantUsages())
	maps.Copy(desired, g.buildDatabaseProtections())
	maps.Copy(desired, g.buildInstanceProtection())
	return desired, nil
}

func isMariaDBDatabaseReady(database v1alpha1.MariaDBDatabase) bool {
	for _, condition := range database.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}

func isMariaDBInstanceReady(pgInstance v1alpha1.MariaDBInstance) bool {
	conditions := pgInstance.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}

func (g *mariaDBUserGenerator) buildUser() map[string]client.Object {
	connSecretName := base.GenerateEligibleKubernetesFullName(g.mariaDBUser.Spec.InstanceRef.Name + "-" + g.mariaDBUser.Name)
	user := &mysqlv1alpha1.User{
		TypeMeta: metav1.TypeMeta{
			Kind:       "User",
			APIVersion: mySqlApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.mariaDBUser.Name,
			Namespace: g.mariaDBUser.Namespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": g.userDisplayName,
			},
			Labels: map[string]string{
				"database.entigo.com/role-name": g.userDisplayName,
			},
		},
		Spec: mysqlv1alpha1.UserSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{Name: connSecretName},
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{
					Kind: "ProviderConfig",
					Name: g.providerConfigName,
				},
			},
			ForProvider: mysqlv1alpha1.UserParameters{
				ResourceOptions: &mysqlv1alpha1.ResourceOptions{
					MaxConnectionsPerHour: g.mariaDBUser.Spec.MaxConnectionsPerHour,
					MaxUserConnections:    g.mariaDBUser.Spec.MaxUserConnections,
					MaxQueriesPerHour:     g.mariaDBUser.Spec.MaxQueriesPerHour,
					MaxUpdatesPerHour:     g.mariaDBUser.Spec.MaxUpdatesPerHour,
				},
			},
		},
	}
	return map[string]client.Object{"user": user}
}

// MariaDB grants are database-scoped only: provider-sql cannot observe a global (*.*) using mySQL provider
func (g *mariaDBUserGenerator) buildGrants() map[string]client.Object {
	grants := make(map[string]client.Object)
	if g.mariaDBUser.Spec.Grant == nil {
		return grants
	}
	if g.mariaDBUser.Spec.DatabaseRef == nil {
		return grants
	}
	for _, user := range g.mariaDBUser.Spec.Grant.Users {
		convertedRoleName := strings.ReplaceAll(user, "_", "-")
		grantName := base.GenerateEligibleKubernetesFullName(grantPrefix + g.mariaDBUser.Name + "-" + convertedRoleName + "-" + g.mariaDBUser.Spec.InstanceRef.Name)
		grant := &mysqlv1alpha1.Grant{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Grant",
				APIVersion: mySqlApiVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      grantName,
				Namespace: g.mariaDBUser.Namespace,
			},
			Spec: mysqlv1alpha1.GrantSpec{
				ManagedResourceSpec: xpv2.ManagedResourceSpec{
					ProviderConfigReference: &xpvcommon.ProviderConfigReference{
						Kind: "ProviderConfig",
						Name: g.providerConfigName,
					},
				},
				ForProvider: mysqlv1alpha1.GrantParameters{
					User: &g.userDisplayName,
				},
			},
		}

		grant.Spec.ForProvider.DatabaseRef = &xpv1.NamespacedReference{
			Name:      g.mariaDBUser.Spec.DatabaseRef.Name,
			Namespace: g.mariaDBUser.Namespace,
		}

		if g.mariaDBUser.Spec.Table != nil {
			grant.Spec.ForProvider.Table = g.mariaDBUser.Spec.Table
		} else {
			grant.Spec.ForProvider.Table = new("*")
		}

		privileges := mysqlv1alpha1.GrantPrivileges{}
		for _, privilege := range g.mariaDBUser.Spec.Privileges {
			privileges = append(privileges, mysqlv1alpha1.GrantPrivilege(privilege))
		}
		grant.Spec.ForProvider.Privileges = privileges

		grants[grantName] = grant
	}
	return grants
}

func (g *mariaDBUserGenerator) buildGrantUsages() map[string]client.Object {
	usages := make(map[string]client.Object)
	if g.mariaDBUser.Spec.Grant == nil || g.mariaDBUser.Spec.DatabaseRef == nil {
		return usages
	}
	for _, user := range g.mariaDBUser.Spec.Grant.Users {
		convertedUserName := strings.ReplaceAll(user, "_", "-")
		grantName := base.GenerateEligibleKubernetesFullName(grantPrefix + g.mariaDBUser.Name + "-" + convertedUserName + "-" + g.mariaDBUser.Spec.InstanceRef.Name)
		usageName := base.GenerateEligibleKubernetesFullName("usage-grant-" + g.mariaDBUser.Name + "-" + convertedUserName + "-" + g.mariaDBUser.Spec.InstanceRef.Name)
		usage := &xpv1beta1.Usage{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Usage",
				APIVersion: crossplaneProtectionApiVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      usageName,
				Namespace: g.mariaDBUser.Namespace,
			},
			Spec: xpv1beta1.UsageSpec{
				ReplayDeletion: new(true),
				Of: xpv1beta1.Resource{
					Kind:       "User",
					APIVersion: mySqlApiVersion,
					ResourceRef: &xpv1beta1.ResourceRef{
						Name: g.mariaDBUser.Name,
					},
				},
				By: &xpv1beta1.Resource{
					Kind:       "Grant",
					APIVersion: mySqlApiVersion,
					ResourceRef: &xpv1beta1.ResourceRef{
						Name: grantName,
					},
				},
			},
		}
		usages[usageName] = usage
	}
	return usages
}

func (g *mariaDBUserGenerator) buildDatabaseProtections() map[string]client.Object {
	protections := make(map[string]client.Object)
	if g.mariaDBUser.Spec.Grant == nil || g.mariaDBUser.Spec.DatabaseRef == nil {
		return protections
	}
	for _, user := range g.mariaDBUser.Spec.Grant.Users {
		convertedUserName := strings.ReplaceAll(user, "_", "-")
		grantName := base.GenerateEligibleKubernetesFullName(grantPrefix + g.mariaDBUser.Name + "-" + convertedUserName + "-" + g.mariaDBUser.Spec.InstanceRef.Name)
		protectionName := base.GenerateEligibleKubernetesFullName("db-protection-" + g.mariaDBUser.Name + "-" + convertedUserName + "-" + g.mariaDBUser.Spec.InstanceRef.Name)
		usage := &xpv1beta1.Usage{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Usage",
				APIVersion: "protection.crossplane.io/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectionName,
				Namespace: g.mariaDBUser.Namespace,
			},
			Spec: xpv1beta1.UsageSpec{
				ReplayDeletion: new(true),
				Of: xpv1beta1.Resource{
					Kind:       "MariaDBDatabase",
					APIVersion: "database.entigo.com/v1alpha1",
					ResourceRef: &xpv1beta1.ResourceRef{
						Name: g.mariaDBUser.Spec.DatabaseRef.Name,
					},
				},
				By: &xpv1beta1.Resource{
					Kind:       "Grant",
					APIVersion: mySqlApiVersion,
					ResourceRef: &xpv1beta1.ResourceRef{
						Name: grantName,
					},
				},
			},
		}
		protections[protectionName] = usage
	}
	return protections
}

func (g *mariaDBUserGenerator) buildInstanceProtection() map[string]client.Object {
	instanceUsages := make(map[string]client.Object)

	usage := &xpv1beta1.Usage{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Usage",
			APIVersion: "protection.crossplane.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.mariaDBUser.Name + "-instance-protection",
			Namespace: g.mariaDBUser.Namespace,
		},
		Spec: xpv1beta1.UsageSpec{
			ReplayDeletion: new(true),
			Of: xpv1beta1.Resource{
				Kind:       "MariaDBInstance",
				APIVersion: "database.entigo.com/v1alpha1",
				ResourceRef: &xpv1beta1.ResourceRef{
					Name: g.mariaDBUser.Spec.InstanceRef.Name,
				},
			},
			By: &xpv1beta1.Resource{
				Kind:       "User",
				APIVersion: mySqlApiVersion,
				ResourceRef: &xpv1beta1.ResourceRef{
					Name: g.mariaDBUser.Name,
				},
			},
		},
	}
	instanceUsages["instance-protection"] = usage
	return instanceUsages
}
