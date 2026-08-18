package main

import (
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	ec2mv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/ec2/v1beta1"
	elasticachemv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/elasticache/v1beta1"
	rdsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/rds/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	databaseEntigoApi  = "database.entigo.com/v1alpha1"
	environmentName    = "platform-apis-database"
	ec2ApiVersion      = "ec2.aws.m.upbound.io/v1beta1"
	instanceProtection = "instance-protection"
)

type GroupImpl struct {
	log logging.Logger
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SetLogger(log logging.Logger) {
	g.log = log
}

func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindPostgreSQL: {
			Instantiate: func() client.Object { return &v1alpha1.PostgreSQLInstance{} },
			Generate:    g.generatePostgreSQL,
		},
		apis.XRKindMariaDBInstance: {
			Instantiate: func() client.Object { return &v1alpha1.MariaDBInstance{} },
			Generate:    g.generateMariaDB,
		},
		apis.XRKindValkey: {
			Instantiate: func() client.Object { return &v1alpha1.ValkeyInstance{} },
			Generate:    g.generateValkeyInstance,
		},
		apis.XRKindPostgreSQLUser: {
			Instantiate: func() client.Object { return &v1alpha1.PostgreSQLUser{} },
			Generate:    g.generatePostgreSQLUser,
		},
		apis.XRKindMariaDBUser: {
			Instantiate: func() client.Object { return &v1alpha1.MariaDBUser{} },
			Generate:    g.generateMariaDBUser,
		},
		apis.XRKindPostgreSQLDatabase: {
			Instantiate: func() client.Object { return &v1alpha1.PostgreSQLDatabase{} },
			Generate:    g.generatePostgreSQLDatabase,
		},
		apis.XRKindMariaDBDatabase: {
			Instantiate: func() client.Object { return &v1alpha1.MariaDBDatabase{} },
			Generate:    g.generateMariaDBDatabase,
		},
	}
}

func (g *GroupImpl) generatePostgreSQL(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GeneratePgInstanceObjects(*obj.(*v1alpha1.PostgreSQLInstance), required, observed)
}

func (g *GroupImpl) generateMariaDB(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateMariaDBInstanceObjects(*obj.(*v1alpha1.MariaDBInstance), required, observed)
}

func (g *GroupImpl) generateValkeyInstance(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateValkeyInstanceObjects(*obj.(*v1alpha1.ValkeyInstance), required, observed)
}

func (g *GroupImpl) generatePostgreSQLUser(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GeneratePgUserObjects(*obj.(*v1alpha1.PostgreSQLUser), required, observed)
}

func (g *GroupImpl) generateMariaDBUser(obj client.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateMariaDBUserObjects(*obj.(*v1alpha1.MariaDBUser), required)
}

func (g *GroupImpl) generatePostgreSQLDatabase(obj client.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GeneratePgDatabaseObjects(*obj.(*v1alpha1.PostgreSQLDatabase), required)
}

func (g *GroupImpl) generateMariaDBDatabase(obj client.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateMariaDBDatabaseObjects(*obj.(*v1alpha1.MariaDBDatabase), required)
}

func (g *GroupImpl) GetSequence(object client.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindPostgreSQLUser:
		return base.NewSequence(true,
			[]string{"role"},
			[]string{"grant-.*"},
			[]string{"usage-grant-.*"},
			[]string{instanceProtection},
		)
	case apis.XRKindMariaDBUser:
		return base.NewSequence(true,
			[]string{"user"},
			[]string{"grant-.*"},
			[]string{"usage-grant-.*"},
			[]string{"db-protection-.*"},
			[]string{instanceProtection},
		)
	case apis.XRKindPostgreSQLDatabase:
		return base.NewSequence(true,
			[]string{"grant-owner-to-dbadmin"},
			[]string{"postgresql-database"},
			[]string{"extension-.*"},
			[]string{"grant-usage"},
			[]string{"owner-protection"},
			[]string{instanceProtection},
		)
	case apis.XRKindMariaDBDatabase:
		return base.NewSequence(true,
			[]string{"mariadb-database"},
			[]string{instanceProtection},
		)
	case apis.XRKindPostgreSQL, apis.XRKindMariaDBInstance:
		name := object.GetName()
		setHash := base.GenerateFNVHash(object.GetUID())
		sg := service.GetSGName(name, setHash)
		sgIngress := service.GetSGIngressName(name, setHash)
		sgEgress := service.GetSGEgressName(name, setHash)
		pc := service.GetPCName(name)
		rdsInstance := service.GetRDSInstanceName(name, setHash)
		es := service.GetESName(name, setHash)
		return base.NewSequence(true, []string{sg, sgIngress, sgEgress, pc, "parameter-group-.*"}, []string{rdsInstance}, []string{es})
	case apis.XRKindValkey:
		return base.NewSequence(true,
			[]string{"security-group", "parameter-group-.*"},
			[]string{"replication-group"},
			[]string{"sg-.*"},
			[]string{"secrets-manager-secret", "credentials"},
			[]string{"secrets-manager-secret-version"},
		)
	default:
		return base.Sequence{}
	}
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	switch observed.GetKind() {
	case "Instance":
		return service.GetRDSInstanceReadyStatus(observed)
	case "ReplicationGroup":
		return service.GetValkeyReplicationGroupReadyStatus(observed)
	case "Database":
		return service.GetResourceReadyStatus(observed)
	case "Grant":
		return service.GetResourceReadyStatus(observed)
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	if compositeResource.GetKind() == apis.XRKindPostgreSQLUser {
		return g.getPostgreSQLUserRequiredResources(compositeResource)
	}
	if compositeResource.GetKind() == apis.XRKindMariaDBUser {
		return g.getMariaDBUserRequiredResources(compositeResource)
	}
	if compositeResource.GetKind() == apis.XRKindPostgreSQLDatabase {
		return g.getPostgreSQLDatabaseRequiredResources(compositeResource)
	}
	if compositeResource.GetKind() == apis.XRKindMariaDBDatabase {
		return g.getMariaDBDatabaseRequiredResources(compositeResource)
	}

	resources := map[string]*fnv1.ResourceSelector{
		base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
	}
	if _, envPresent := required[base.EnvironmentKey]; !envPresent {
		return resources, nil
	}
	env, err := service.GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	switch compositeResource.GetKind() {
	case apis.XRKindPostgreSQL, apis.XRKindMariaDBInstance:
		secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", compositeResource.GetName(), "dbadmin"))
		secretNamespace := compositeResource.GetNamespace()
		resources["VPC"] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: ec2ApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.VPC},
			Namespace:  &env.AWSProvider,
		}
		resources["KMSDataKey"] = base.RequiredKMSKey(env.DataKMSKey, env.AWSProvider)
		resources["KMSConfigKey"] = base.RequiredKMSKey(env.ConfigKMSKey, env.AWSProvider)
		resources["DBSubnetGroup"] = &fnv1.ResourceSelector{
			Kind:       "SubnetGroup",
			ApiVersion: "rds.aws.m.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.SubnetGroup},
			Namespace:  &env.AWSProvider,
		}
		resources["Secret"] = &fnv1.ResourceSelector{
			Kind:       "Secret",
			ApiVersion: "v1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: secretName},
			Namespace:  &secretNamespace,
		}
	case apis.XRKindValkey:
		resources[service.VPCKey] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: ec2ApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.VPC},
			Namespace:  &env.AWSProvider,
		}
		resources[service.ElasticacheSubnetGroupKey] = &fnv1.ResourceSelector{
			Kind:       "SubnetGroup",
			ApiVersion: "elasticache.aws.m.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.ElasticacheSubnetGroup},
			Namespace:  &env.AWSProvider,
		}
		resources["KMSDataKey"] = base.RequiredKMSKey(env.DataKMSKey, env.AWSProvider)
		resources["KMSConfigKey"] = base.RequiredKMSKey(env.ConfigKMSKey, env.AWSProvider)
		resources[service.ComputeSubnetsKey] = &fnv1.ResourceSelector{
			Kind:       "Subnet",
			ApiVersion: ec2ApiVersion,
			Match: &fnv1.ResourceSelector_MatchLabels{
				MatchLabels: &fnv1.MatchLabels{
					Labels: map[string]string{"subnet-type": "compute"},
				},
			},
			Namespace: &env.AWSProvider,
		}
	}
	return resources, nil
}

func (g *GroupImpl) getPostgreSQLDatabaseRequiredResources(compositeResource *composite.Unstructured) (map[string]*fnv1.ResourceSelector, error) {
	owner, found, err := unstructured.NestedString(compositeResource.Object, "spec", "owner")
	if err != nil || !found || owner == "" {
		return nil, fmt.Errorf("cannot get spec.owner from PostgreSQLDatabase %s", compositeResource.GetName())
	}
	namespace := compositeResource.GetNamespace()
	return map[string]*fnv1.ResourceSelector{
		"OwnerRole": {
			Kind:       "Role",
			ApiVersion: "postgresql.sql.m.crossplane.io/v1alpha1",
			Match: &fnv1.ResourceSelector_MatchLabels{
				MatchLabels: &fnv1.MatchLabels{
					Labels: map[string]string{"database.entigo.com/role-name": owner},
				},
			},
			Namespace: &namespace,
		},
	}, nil
}

func (g *GroupImpl) getPostgreSQLUserRequiredResources(compositeResource *composite.Unstructured) (map[string]*fnv1.ResourceSelector, error) {
	instanceName, found, err := unstructured.NestedString(compositeResource.Object, "spec", "instanceRef", "name")
	if err != nil || !found || instanceName == "" {
		return nil, fmt.Errorf("cannot get spec.instanceRef.name from PostgreSQLUser %s", compositeResource.GetName())
	}
	namespace := compositeResource.GetNamespace()
	return map[string]*fnv1.ResourceSelector{
		"PostgreSQLInstance": {
			Kind:       "PostgreSQLInstance",
			ApiVersion: databaseEntigoApi,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: instanceName},
			Namespace:  &namespace,
		},
	}, nil
}

func (g *GroupImpl) getMariaDBUserRequiredResources(compositeResource *composite.Unstructured) (map[string]*fnv1.ResourceSelector, error) {
	instanceName, found, err := unstructured.NestedString(compositeResource.Object, "spec", "instanceRef", "name")
	if err != nil || !found || instanceName == "" {
		return nil, fmt.Errorf("cannot get spec.instanceRef.name from MariaDBUser %s", compositeResource.GetName())
	}
	namespace := compositeResource.GetNamespace()
	resources := map[string]*fnv1.ResourceSelector{
		"MariaDBInstance": {
			Kind:       "MariaDBInstance",
			ApiVersion: databaseEntigoApi,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: instanceName},
			Namespace:  &namespace,
		},
	}

	databaseName, found, err := unstructured.NestedString(compositeResource.Object, "spec", "databaseRef", "name")
	if err != nil || !found || databaseName == "" {
		return nil, fmt.Errorf("cannot get spec.databaseRef.name from MariaDBUser %s", compositeResource.GetName())
	}
	resources["MariaDBDatabase"] = &fnv1.ResourceSelector{
		Kind:       "Database",
		ApiVersion: "mysql.sql.m.crossplane.io/v1alpha1",
		Match: &fnv1.ResourceSelector_MatchLabels{
			MatchLabels: &fnv1.MatchLabels{
				Labels: map[string]string{"database.entigo.com/database-name": databaseName},
			},
		},
		Namespace: &namespace,
	}
	return resources, nil
}

func (g *GroupImpl) getMariaDBDatabaseRequiredResources(compositeResource *composite.Unstructured) (map[string]*fnv1.ResourceSelector, error) {
	instanceName, found, err := unstructured.NestedString(compositeResource.Object, "spec", "instanceRef", "name")
	if err != nil || !found || instanceName == "" {
		return nil, fmt.Errorf("cannot get spec.instanceRef.name from MariaDBDatabase %s", compositeResource.GetName())
	}
	namespace := compositeResource.GetNamespace()
	return map[string]*fnv1.ResourceSelector{
		"MariaDBInstance": {
			Kind:       "MariaDBInstance",
			ApiVersion: databaseEntigoApi,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: instanceName},
			Namespace:  &namespace,
		},
	}, nil
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	switch {
	case observed.GetKind() == "Instance" && strings.HasPrefix(observed.GetAPIVersion(), "rds.aws.m.upbound.io"):
		return getDBInstanceStatus(observed)
	case observed.GetKind() == "ReplicationGroup" && strings.HasPrefix(observed.GetAPIVersion(), "elasticache.aws.m.upbound.io"):
		return getReplicationGroupStatus(observed)
	case observed.GetKind() == "SecurityGroup" && strings.HasPrefix(observed.GetAPIVersion(), "ec2.aws.m.upbound.io"):
		return getSecurityGroupStatus(observed)
	default:
		return nil, nil
	}
}

func getDBInstanceStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	var dbInstance rdsmv1beta1.Instance
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observed.Object, &dbInstance); err != nil {
		return nil, fmt.Errorf("cannot convert Instance object to RDS Instance: %w", err)
	}
	if dbInstance.Spec.ForProvider.Engine != nil && *dbInstance.Spec.ForProvider.Engine == "mariadb" {
		mariaDBStatus := service.GetMariaDBStatusFromDbInstance(dbInstance)
		return runtime.DefaultUnstructuredConverter.ToUnstructured(&mariaDBStatus)
	}

	postgreSQLStatus := service.GetPostgreSQLStatusFromDbInstance(dbInstance)
	return runtime.DefaultUnstructuredConverter.ToUnstructured(&postgreSQLStatus)
}

func getReplicationGroupStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	var rg elasticachemv1beta1.ReplicationGroup
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observed.Object, &rg); err != nil {
		return nil, fmt.Errorf("cannot convert ReplicationGroup: %w", err)
	}
	status := service.GetValkeyStatusFromReplicationGroup(rg)
	return runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
}

func getSecurityGroupStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	annotations := observed.GetAnnotations()
	if annotations == nil || annotations["crossplane.io/composition-resource-name"] != "security-group" {
		return nil, nil
	}

	var sg ec2mv1beta1.SecurityGroup
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observed.Object, &sg); err != nil {
		return nil, fmt.Errorf("cannot convert SecurityGroup: %w", err)
	}
	sgStatus := service.GetValkeySecurityGroupStatus(sg)
	return runtime.DefaultUnstructuredConverter.ToUnstructured(&map[string]interface{}{
		"securityGroup": sgStatus,
	})
}

func (g *GroupImpl) PostProcessStatus(status map[string]interface{}, observed map[resource.Name]resource.ObservedComposed) (map[string]interface{}, error) {
	// Aggregate SecurityGroupRules into securityGroup.rules
	sgInterface, ok := status["securityGroup"]
	if !ok {
		return status, nil
	}

	sg, ok := sgInterface.(map[string]interface{})
	if !ok {
		return status, nil
	}

	var rules []interface{}
	for _, observedResource := range observed {
		res := observedResource.Resource
		if res.GetKind() != "SecurityGroupRule" || !strings.HasPrefix(res.GetAPIVersion(), "ec2.aws.m.upbound.io") {
			continue
		}

		var sgr ec2mv1beta1.SecurityGroupRule
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &sgr); err != nil {
			continue
		}

		rule := service.GetValkeySecurityGroupRuleStatus(sgr)
		ruleMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&rule)
		if err != nil {
			continue
		}
		rules = append(rules, ruleMap)
	}

	if len(rules) > 0 {
		sg["rules"] = rules
	}

	return status, nil
}
