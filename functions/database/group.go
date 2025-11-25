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
	rdsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/rds/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	environmentName = "platform-apis-database"
)

type GroupImpl struct {
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindPostgreSQL: {
			Instantiate: func() runtime.Object { return &v1alpha1.PostgreSQLInstance{} },
			Generate:    g.generatePostgreSQL,
		},
	}
}

func (g *GroupImpl) generatePostgreSQL(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GeneratePgInstanceObjects(*obj.(*v1alpha1.PostgreSQLInstance), required, observed)
}

func (g *GroupImpl) GetSequence(object runtime.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindPostgreSQL:
		instance := *object.(*v1alpha1.PostgreSQLInstance)
		setHash := base.GenerateFNVHash(instance.GetUID())
		sg := service.GetSGName(instance.GetName(), setHash)
		sgIngress := service.GetSGIngressName(instance.GetName(), setHash)
		sgEgress := service.GetSGEgressName(instance.GetName(), setHash)
		rdsInstance := service.GetRDSInstanceName(instance.GetName(), setHash)
		es := service.GetESName(instance.GetName(), setHash)
		return base.NewSequence(false, []string{sg, sgIngress, sgEgress}, []string{rdsInstance}, []string{es})
	default:
		return base.Sequence{}
	}
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	switch observed.GetKind() {
	case "Instance":
		return service.GetRDSInstanceReadyStatus(observed)
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindPostgreSQL:
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
		secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", compositeResource.GetName(), "dbadmin"))
		secretNamespace := compositeResource.GetNamespace()
		resources["VPC"] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: "ec2.aws.m.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.VPC},
			Namespace:  &env.AWSProvider,
		}
		resources["KMSDataKey"] = &fnv1.ResourceSelector{
			Kind:       "Key",
			ApiVersion: "kms.aws.m.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.DataKMSKey},
			Namespace:  &env.AWSProvider,
		}
		resources["KMSConfigKey"] = &fnv1.ResourceSelector{
			Kind:       "Key",
			ApiVersion: "kms.aws.m.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.ConfigKMSKey},
			Namespace:  &env.AWSProvider,
		}
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
		return resources, nil
	default:
		return nil, nil
	}
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	switch {
	case observed.GetKind() == "Instance" && strings.HasPrefix(observed.GetAPIVersion(), "rds.aws.m.upbound.io"):
		return getDBInstanceStatus(observed)
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
