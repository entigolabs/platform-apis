package service

import (
	"fmt"
	"maps"
	"strings"
	"time"

	postgresv1alpha1 "github.com/crossplane-contrib/provider-sql/apis/namespaced/postgresql/v1alpha1"
	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	xpv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type pgUserGenerator struct {
	pgUser             v1alpha1.PostgreSQLUser
	pgInstance         v1alpha1.PostgreSQLInstance
	observed           map[resource.Name]resource.ObservedComposed
	providerConfigName string
	roleDisplayName    string
}

func GeneratePgUserObjects(
	pgUser v1alpha1.PostgreSQLUser,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]client.Object, error) {
	g, err := newPgUserGenerator(pgUser, required, observed)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newPgUserGenerator(
	pgUser v1alpha1.PostgreSQLUser,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*pgUserGenerator, error) {
	var pgInstance v1alpha1.PostgreSQLInstance
	if err := base.ExtractRequiredResource(required, "PostgreSQLInstance", &pgInstance); err != nil {
		return nil, err
	}

	roleDisplayName := pgUser.Name
	if pgUser.Spec.Name != "" {
		roleDisplayName = pgUser.Spec.Name
	}

	return &pgUserGenerator{
		pgUser:             pgUser,
		pgInstance:         pgInstance,
		observed:           observed,
		providerConfigName: pgUser.Spec.InstanceRef.Name + "-providerconfig",
		roleDisplayName:    roleDisplayName,
	}, nil
}

func (g *pgUserGenerator) generate() (map[string]client.Object, error) {
	desired := make(map[string]client.Object)
	instanceReady := isPgInstanceReady(g.pgInstance)
	if !instanceReady {
		return desired, fmt.Errorf("temporarily waiting for PostgreSQLInstance %s to become ready", g.pgInstance.Name)
	}

	maps.Copy(desired, g.buildRole())
	maps.Copy(desired, g.buildGrants())
	maps.Copy(desired, g.buildGrantUsages())
	maps.Copy(desired, g.buildInstanceProtection())
	return desired, nil
}

func isPgInstanceReady(pgInstance v1alpha1.PostgreSQLInstance) bool {
	conditions := pgInstance.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}

func (g *pgUserGenerator) buildRole() map[string]client.Object {
	connSecretName := base.GenerateEligibleKubernetesFullName(g.pgUser.Spec.InstanceRef.Name + "-" + g.pgUser.Name)
	role := &postgresv1alpha1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: pgSqlApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.pgUser.Name,
			Namespace: g.pgUser.Namespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": g.roleDisplayName,
			},
			Labels: map[string]string{
				"database.entigo.com/role-name": g.roleDisplayName,
			},
		},
		Spec: postgresv1alpha1.RoleSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{Name: connSecretName},
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{
					Kind: "ProviderConfig",
					Name: g.providerConfigName,
				},
			},
			ForProvider: postgresv1alpha1.RoleParameters{
				// Set every field lateInit() checks, so provider-sql does not
				// late-initialize them and write the spec back on first observation.
				ConnectionLimit: new(int32(-1)),
				Privileges: postgresv1alpha1.RolePrivilege{
					Login:       &g.pgUser.Spec.Login,
					CreateDb:    &g.pgUser.Spec.CreateDb,
					CreateRole:  &g.pgUser.Spec.CreateRole,
					Inherit:     &g.pgUser.Spec.Inherit,
					SuperUser:   new(false),
					Replication: new(false),
					BypassRls:   new(false),
				},
			},
		},
	}

	role.Spec.ForProvider.PasswordRotationTrigger = g.passwordRotationTrigger()

	return map[string]client.Object{"role": role}
}

func (g *pgUserGenerator) passwordRotationTrigger() *metav1.Time {
	observedRole, found := g.observed["role"]
	if !found {
		return nil
	}

	if trigger := getObservedTime(observedRole.Resource, "spec", "forProvider", "passwordRotationTrigger"); trigger != nil {
		return trigger
	}

	if g.pgInstance.Spec.SnapshotIdentifier == "" {
		return nil
	}

	lastPasswordChange := getObservedTime(observedRole.Resource, "status", "atProvider", "lastPasswordChange")
	if lastPasswordChange == nil {
		return nil
	}

	return new(metav1.NewTime(lastPasswordChange.Add(time.Second)))
}

func getObservedTime(observed *composed.Unstructured, fields ...string) *metav1.Time {
	value, found, err := unstructured.NestedString(observed.Object, fields...)
	if err != nil || !found || value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return new(metav1.NewTime(parsed))
}

func (g *pgUserGenerator) buildGrants() map[string]client.Object {
	grants := make(map[string]client.Object)
	if g.pgUser.Spec.Grant == nil {
		return grants
	}
	for _, role := range g.pgUser.Spec.Grant.Roles {
		convertedRoleName := strings.ReplaceAll(role, "_", "-")
		grantName := base.GenerateEligibleKubernetesFullName("grant-" + g.pgUser.Name + "-" + convertedRoleName + "-" + g.pgUser.Spec.InstanceRef.Name)
		grant := &postgresv1alpha1.Grant{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Grant",
				APIVersion: pgSqlApiVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      grantName,
				Namespace: g.pgUser.Namespace,
			},
			Spec: postgresv1alpha1.GrantSpec{
				ManagedResourceSpec: xpv2.ManagedResourceSpec{
					ProviderConfigReference: &xpvcommon.ProviderConfigReference{
						Kind: "ProviderConfig",
						Name: g.providerConfigName,
					},
				},
				ForProvider: postgresv1alpha1.GrantParameters{
					Role:     &g.roleDisplayName,
					MemberOf: new(role),
				},
			},
		}

		grants[grantName] = grant
	}
	return grants
}

func (g *pgUserGenerator) buildGrantUsages() map[string]client.Object {
	usages := make(map[string]client.Object)
	if g.pgUser.Spec.Grant == nil {
		return usages
	}
	for _, role := range g.pgUser.Spec.Grant.Roles {
		convertedRoleName := strings.ReplaceAll(role, "_", "-")
		grantName := base.GenerateEligibleKubernetesFullName("grant-" + g.pgUser.Name + "-" + convertedRoleName + "-" + g.pgUser.Spec.InstanceRef.Name)
		usageName := base.GenerateEligibleKubernetesFullName("usage-grant-" + g.pgUser.Name + "-" + convertedRoleName + "-" + g.pgUser.Spec.InstanceRef.Name)
		usage := &xpv1beta1.Usage{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Usage",
				APIVersion: crossplaneProtectionApiVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      usageName,
				Namespace: g.pgUser.Namespace,
			},
			Spec: xpv1beta1.UsageSpec{
				ReplayDeletion: new(true),
				Of: xpv1beta1.Resource{
					Kind:       "Role",
					APIVersion: pgSqlApiVersion,
					ResourceRef: &xpv1beta1.ResourceRef{
						Name: g.pgUser.Name,
					},
				},
				By: &xpv1beta1.Resource{
					Kind:       "Grant",
					APIVersion: pgSqlApiVersion,
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

func (g *pgUserGenerator) buildInstanceProtection() map[string]client.Object {
	instanceUsages := make(map[string]client.Object)

	usage := &xpv1beta1.Usage{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Usage",
			APIVersion: crossplaneProtectionApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-" + g.pgUser.Name + "-instance-protection",
			Namespace: g.pgUser.Namespace,
		},
		Spec: xpv1beta1.UsageSpec{
			ReplayDeletion: new(true),
			Of: xpv1beta1.Resource{
				Kind:       "PostgreSQLInstance",
				APIVersion: "database.entigo.com/v1alpha1",
				ResourceRef: &xpv1beta1.ResourceRef{
					Name: g.pgUser.Spec.InstanceRef.Name,
				},
			},
			By: &xpv1beta1.Resource{
				Kind:       "Role",
				APIVersion: pgSqlApiVersion,
				ResourceRef: &xpv1beta1.ResourceRef{
					Name: g.pgUser.Name,
				},
			},
		},
	}
	instanceUsages["instance-protection"] = usage
	return instanceUsages
}
