package service

import (
	"fmt"
	"maps"

	mysqlv1alpha1 "github.com/crossplane-contrib/provider-sql/apis/namespaced/mysql/v1alpha1"
	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	xpv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mariaDBDatabaseGenerator struct {
	mariaDBDatabase    v1alpha1.MariaDBDatabase
	mariaDBInstance    v1alpha1.MariaDBInstance
	providerConfigName string
	dbDisplayName      string
}

func GenerateMariaDBDatabaseObjects(
	mariaDBDatabase v1alpha1.MariaDBDatabase,
	required map[string][]resource.Required,
) (map[string]client.Object, error) {
	g, err := newMariaDBDatabaseGenerator(mariaDBDatabase, required)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newMariaDBDatabaseGenerator(
	mariaDBDatabase v1alpha1.MariaDBDatabase,
	required map[string][]resource.Required,
) (*mariaDBDatabaseGenerator, error) {
	var mariaDBInstance v1alpha1.MariaDBInstance
	if err := base.ExtractRequiredResource(required, "MariaDBInstance", &mariaDBInstance); err != nil {
		return nil, err
	}

	dbDisplayName := mariaDBDatabase.Name
	if mariaDBDatabase.Spec.Name != "" {
		dbDisplayName = mariaDBDatabase.Spec.Name
	}

	return &mariaDBDatabaseGenerator{
		mariaDBDatabase:    mariaDBDatabase,
		mariaDBInstance:    mariaDBInstance,
		providerConfigName: mariaDBDatabase.Spec.InstanceRef.Name + "-providerconfig",
		dbDisplayName:      dbDisplayName,
	}, nil
}

func (g *mariaDBDatabaseGenerator) generate() (map[string]client.Object, error) {
	desired := make(map[string]client.Object)
	if !isMariaDBInstanceReady(g.mariaDBInstance) {
		return desired, fmt.Errorf("temporarily waiting for MariaDBInstance %s to become ready", g.mariaDBInstance.Name)
	}

	maps.Copy(desired, g.buildDatabase())
	if g.mariaDBDatabase.Spec.DeletionProtection {
		maps.Copy(desired, g.buildInstanceProtection())
	}
	return desired, nil
}

func (g *mariaDBDatabaseGenerator) buildDatabase() map[string]client.Object {
	db := &mysqlv1alpha1.Database{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Database",
			APIVersion: mySqlApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.mariaDBDatabase.Name,
			Namespace: g.mariaDBDatabase.Namespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": g.dbDisplayName,
			},
		},
		Spec: mysqlv1alpha1.DatabaseSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{
					Kind: "ProviderConfig",
					Name: g.providerConfigName,
				},
			},
			ForProvider: mysqlv1alpha1.DatabaseParameters{
				DefaultCollation:    g.mariaDBDatabase.Spec.DefaultCollation,
				DefaultCharacterSet: g.mariaDBDatabase.Spec.DefaultCharacterSet,
			},
		},
	}
	return map[string]client.Object{"mariadb-database": db}
}

func (g *mariaDBDatabaseGenerator) buildInstanceProtection() map[string]client.Object {
	replayDeletion := true
	usage := &xpv1beta1.Usage{
		TypeMeta: metav1.TypeMeta{Kind: "Usage", APIVersion: CrossplaneProtectionApi},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.mariaDBDatabase.Name + "-instance-protection",
			Namespace: g.mariaDBDatabase.Namespace,
		},
		Spec: xpv1beta1.UsageSpec{
			ReplayDeletion: &replayDeletion,
			Of: xpv1beta1.Resource{
				Kind:        "MariaDBInstance",
				APIVersion:  "database.entigo.com/v1alpha1",
				ResourceRef: &xpv1beta1.ResourceRef{Name: g.mariaDBDatabase.Spec.InstanceRef.Name},
			},
			By: &xpv1beta1.Resource{
				Kind:        "Database",
				APIVersion:  mySqlApiVersion,
				ResourceRef: &xpv1beta1.ResourceRef{Name: g.mariaDBDatabase.Name},
			},
		},
	}
	return map[string]client.Object{"instance-protection": usage}
}
