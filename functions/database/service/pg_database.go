package service

import (
	"fmt"
	"maps"
	"strings"

	postgresv1alpha1 "github.com/crossplane-contrib/provider-sql/apis/namespaced/postgresql/v1alpha1"
	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	xpv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type pgDatabaseGenerator struct {
	pgDatabase         v1alpha1.PostgreSQLDatabase
	ownerRole          postgresv1alpha1.Role
	providerConfigName string
}

func GeneratePgDatabaseObjects(
	pgDatabase v1alpha1.PostgreSQLDatabase,
	required map[string][]resource.Required,
) (map[string]runtime.Object, error) {
	g, err := newPgDatabaseGenerator(pgDatabase, required)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newPgDatabaseGenerator(
	pgDatabase v1alpha1.PostgreSQLDatabase,
	required map[string][]resource.Required,
) (*pgDatabaseGenerator, error) {
	var ownerRole postgresv1alpha1.Role
	if err := base.ExtractRequiredResource(required, "OwnerRole", &ownerRole); err != nil {
		return nil, err
	}

	return &pgDatabaseGenerator{
		pgDatabase:         pgDatabase,
		ownerRole:          ownerRole,
		providerConfigName: pgDatabase.Spec.InstanceRef.Name + "-providerconfig",
	}, nil
}

func (g *pgDatabaseGenerator) generate() (map[string]runtime.Object, error) {
	desired := make(map[string]runtime.Object)
	roleReady := isRoleReady(g.ownerRole)
	if !roleReady {
		return desired, fmt.Errorf("temporarily waiting for Owner Role %s to become ready", g.ownerRole.Name)
	}

	maps.Copy(desired, g.buildGrant())
	maps.Copy(desired, g.buildDatabase())
	maps.Copy(desired, g.buildExtensions())
	maps.Copy(desired, g.buildInstanceProtection())
	return desired, nil
}

func isRoleReady(role postgresv1alpha1.Role) bool {
	for _, condition := range role.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}

func (g *pgDatabaseGenerator) buildGrant() map[string]runtime.Object {
	grantName := g.pgDatabase.Name + "-grant-owner-to-dbadmin"
	dbAdmin := "dbadmin"
	owner := g.pgDatabase.Spec.Owner
	grant := &postgresv1alpha1.Grant{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Grant",
			APIVersion: pgSqlApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      grantName,
			Namespace: g.pgDatabase.Namespace,
		},
		Spec: postgresv1alpha1.GrantSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{
					Kind: "ProviderConfig",
					Name: g.providerConfigName,
				},
			},
			ForProvider: postgresv1alpha1.GrantParameters{
				Role:     &dbAdmin,
				MemberOf: &owner,
			},
		},
	}
	return map[string]runtime.Object{"grant-owner-to-dbadmin": grant}
}

func (g *pgDatabaseGenerator) buildDatabase() map[string]runtime.Object {
	owner := g.pgDatabase.Spec.Owner
	var encoding, lcCType, lcCollate, template *string
	if g.pgDatabase.Spec.Encoding != "" {
		encoding = &g.pgDatabase.Spec.Encoding
	}
	if g.pgDatabase.Spec.LCCType != "" {
		lcCType = &g.pgDatabase.Spec.LCCType
	}
	if g.pgDatabase.Spec.LCCollate != "" {
		lcCollate = &g.pgDatabase.Spec.LCCollate
	}
	if g.pgDatabase.Spec.DBTemplate != "" {
		template = &g.pgDatabase.Spec.DBTemplate
	}
	db := &postgresv1alpha1.Database{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Database",
			APIVersion: pgSqlApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.pgDatabase.Name,
			Namespace: g.pgDatabase.Namespace,
		},
		Spec: postgresv1alpha1.DatabaseSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{
					Kind: "ProviderConfig",
					Name: g.providerConfigName,
				},
			},
			ForProvider: postgresv1alpha1.DatabaseParameters{
				Owner:     &owner,
				Encoding:  encoding,
				LCCType:   lcCType,
				LCCollate: lcCollate,
				Template:  template,
			},
		},
	}
	return map[string]runtime.Object{"postgresql-database": db}
}

func (g *pgDatabaseGenerator) buildExtensions() map[string]runtime.Object {
	extensions := make(map[string]runtime.Object)
	dbName := g.pgDatabase.Name
	for i, extensionName := range g.pgDatabase.Spec.Extensions {
		convertedName := strings.ReplaceAll(extensionName, "_", "-")
		resourceName := fmt.Sprintf("extension-%s-%d", convertedName, i)

		extension := &postgresv1alpha1.Extension{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Extension",
				APIVersion: pgSqlApiVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      g.pgDatabase.Name + "-" + convertedName,
				Namespace: g.pgDatabase.Namespace,
			},
			Spec: postgresv1alpha1.ExtensionSpec{
				ManagedResourceSpec: xpv2.ManagedResourceSpec{
					ProviderConfigReference: &xpvcommon.ProviderConfigReference{
						Kind: "ProviderConfig",
						Name: g.providerConfigName,
					},
				},
				ForProvider: postgresv1alpha1.ExtensionParameters{
					Extension: extensionName,
					Database:  &dbName,
				},
			},
		}

		if g.pgDatabase.Spec.ExtensionConfig != nil {
			if cfg, ok := g.pgDatabase.Spec.ExtensionConfig[extensionName]; ok && cfg.Schema != "" {
				schema := cfg.Schema
				extension.Spec.ForProvider.Schema = &schema
			}
		}

		extensions[resourceName] = extension
	}
	return extensions
}

func GetPgDatabaseDatabaseReadyStatus(observed *composed.Unstructured) resource.Ready {
	if isResourceReady(observed) {
		return resource.ReadyTrue
	}
	return resource.ReadyFalse
}

func (g *pgDatabaseGenerator) buildInstanceProtection() map[string]runtime.Object {
	replayDeletion := true
	usage := &xpv1beta1.Usage{
		TypeMeta: metav1.TypeMeta{Kind: "Usage", APIVersion: "protection.crossplane.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.pgDatabase.Name + "-instance-protection",
			Namespace: g.pgDatabase.Namespace,
		},
		Spec: xpv1beta1.UsageSpec{
			ReplayDeletion: &replayDeletion,
			Of: xpv1beta1.Resource{
				Kind:        "PostgreSQLInstance",
				APIVersion:  "database.entigo.com/v1alpha1",
				ResourceRef: &xpv1beta1.ResourceRef{Name: g.pgDatabase.Spec.InstanceRef.Name},
			},
			By: &xpv1beta1.Resource{
				Kind:        "Database",
				APIVersion:  pgSqlApiVersion,
				ResourceRef: &xpv1beta1.ResourceRef{Name: g.pgDatabase.Name},
			},
		},
	}
	return map[string]runtime.Object{"instance-protection": usage}
}
