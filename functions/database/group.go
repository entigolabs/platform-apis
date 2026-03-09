package main

import (
	"fmt"
	"strings"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	ec2mv1beta1 "github.com/upbound/provider-aws/apis/namespaced/ec2/v1beta1"
	elasticachemv1beta1 "github.com/upbound/provider-aws/apis/namespaced/elasticache/v1beta1"
	rdsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/rds/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	environmentName = "platform-apis-database"
)

type GroupImpl struct{}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindPostgreSQL: {
			Instantiate: func() runtime.Object { return &v1alpha1.PostgreSQLInstance{} },
			Generate:    g.generatePostgreSQL,
		},
		apis.XRKindValkey: {
			Instantiate: func() runtime.Object { return &v1alpha1.ValkeyInstance{} },
			Generate:    g.generateValkeyInstance,
		},
		apis.XRKindPostgreSQLUser: {
			Instantiate: func() runtime.Object { return &v1alpha1.PostgreSQLUser{} },
			Generate:    g.generatePostgreSQLUser,
		},
		apis.XRKindPostgreSQLDatabase: {
			Instantiate: func() runtime.Object { return &v1alpha1.PostgreSQLDatabase{} },
			Generate:    g.generatePostgreSQLDatabase,
		},
	}
}

func (g *GroupImpl) generatePostgreSQL(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GeneratePgInstanceObjects(*obj.(*v1alpha1.PostgreSQLInstance), required, observed)
}

func (g *GroupImpl) generateValkeyInstance(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateValkeyInstanceObjects(*obj.(*v1alpha1.ValkeyInstance), required, observed)
}

func (g *GroupImpl) generatePostgreSQLUser(obj runtime.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GeneratePgUserObjects(*obj.(*v1alpha1.PostgreSQLUser), required)
}

func (g *GroupImpl) generatePostgreSQLDatabase(obj runtime.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GeneratePgDatabaseObjects(*obj.(*v1alpha1.PostgreSQLDatabase), required)
}

func (g *GroupImpl) GetSequence(object runtime.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindPostgreSQLUser:
		return base.NewSequence(true,
			[]string{"role"},
			[]string{"grant-.*"},
			[]string{"usage-grant-.*"},
			[]string{"instance-protection"},
		)
	case apis.XRKindPostgreSQLDatabase:
		return base.NewSequence(true,
			[]string{"grant-owner-to-dbadmin"},
			[]string{"postgresql-database"},
			[]string{"extension-.*"},
			[]string{"instance-protection"},
		)
	case apis.XRKindPostgreSQL:
		instance := *object.(*v1alpha1.PostgreSQLInstance)
		setHash := base.GenerateFNVHash(instance.GetUID())
		sg := service.GetSGName(instance.GetName(), setHash)
		sgIngress := service.GetSGIngressName(instance.GetName(), setHash)
		sgEgress := service.GetSGEgressName(instance.GetName(), setHash)
		rdsInstance := service.GetRDSInstanceName(instance.GetName(), setHash)
		es := service.GetESName(instance.GetName(), setHash)
		pc := service.GetPCName(instance.GetName())
		return base.NewSequence(false, []string{sg, sgIngress, sgEgress}, []string{rdsInstance}, []string{es, pc})
	case apis.XRKindValkey:
		return base.NewSequence(true,
			[]string{"security-group"},
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
		return service.GetPgDatabaseDatabaseReadyStatus(observed)
	case "Grant":
		return service.GetPgUserGrantReadyStatus(observed)
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	if compositeResource.GetKind() == apis.XRKindPostgreSQLUser {
		return g.getPostgreSQLUserRequiredResources(compositeResource)
	}
	if compositeResource.GetKind() == apis.XRKindPostgreSQLDatabase {
		return g.getPostgreSQLDatabaseRequiredResources(compositeResource)
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
	case apis.XRKindPostgreSQL:
		secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", compositeResource.GetName(), "dbadmin"))
		secretNamespace := compositeResource.GetNamespace()
		resources["VPC"] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: "ec2.aws.m.upbound.io/v1beta1",
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
			ApiVersion: "ec2.aws.m.upbound.io/v1beta1",
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
			ApiVersion: "ec2.aws.m.upbound.io/v1beta1",
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
			ApiVersion: "database.entigo.com/v1alpha1",
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
