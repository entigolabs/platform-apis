package main

import (
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/model/v1alpha1"
	rdsv1beta3 "github.com/upbound/provider-aws/apis/rds/v1beta3"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	XRKindPostgreSQL = "PostgreSQL"
)

type GroupImpl struct {
	awsProvider string
}

func NewGroupImpl(awsProvider string) base.GroupService {
	return &GroupImpl{
		awsProvider: awsProvider,
	}
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		XRKindPostgreSQL: {
			Instantiate: func() runtime.Object { return &v1alpha1.PostgreSQL{} },
			Generate:    g.generatePostgreSQL,
		},
	}
}

func (g *GroupImpl) generatePostgreSQL(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return GeneratePostgreSQLObjects(*obj.(*v1alpha1.PostgreSQL), required, g.awsProvider, observed)
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	switch observed.GetKind() {
	case "Instance":
		return GetInstanceReadyStatus(observed)
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(kind string) map[string]*fnv1.ResourceSelector {
	switch kind {
	case XRKindPostgreSQL:
		return map[string]*fnv1.ResourceSelector{
			"VPC": {
				Kind:       "VPC",
				ApiVersion: "ec2.aws.upbound.io/v1beta1",
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}},
			},
			"KMSKey": {
				Kind:       "Key",
				ApiVersion: "kms.aws.upbound.io/v1beta1",
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}},
			},
			"DBSubnetGroup": {
				Kind:       "SubnetGroup",
				ApiVersion: "rds.aws.upbound.io/v1beta1",
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}},
			},
		}
	default:
		return nil
	}
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	switch {
	case observed.GetKind() == "Instance" && strings.HasPrefix(observed.GetAPIVersion(), "rds.aws.upbound.io"):
		return getDBInstanceStatus(observed)
	default:
		return nil, nil
	}
}

func getDBInstanceStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	var dbInstance rdsv1beta3.Instance
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observed.Object, &dbInstance); err != nil {
		return nil, fmt.Errorf("cannot convert Instance object to RDS Instance: %w", err)
	}
	postgreSQLStatus := GetPostgreSQLStatusFromDbInstance(dbInstance)

	return runtime.DefaultUnstructuredConverter.ToUnstructured(&postgreSQLStatus)
}

func (g *GroupImpl) AddStatusConditions(compositeResource *composite.Unstructured, observed map[resource.Name]resource.ObservedComposed) {
	if compositeResource.GetKind() == XRKindPostgreSQL {
		esName := resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", compositeResource.GetName(), "es")))
		esObserved, esExists := observed[esName]
		if esExists && base.GetCrossplaneReadyStatus(esObserved.Resource) == resource.ReadyTrue {
			compositeResource.SetConditions(xpv1.Available())
		} else {
			compositeResource.SetConditions(xpv1.Creating())
		}
	}
}
