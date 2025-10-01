package main

import (
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/model/v1alpha1"
	rdsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/rds/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	XRKindPostgreSQL = "PostgreSQLInstance"
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
			Instantiate: func() runtime.Object { return &v1alpha1.PostgreSQLInstance{} },
			Generate:    g.generatePostgreSQL,
		},
	}
}

func (g *GroupImpl) generatePostgreSQL(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return GeneratePgInstanceObjects(*obj.(*v1alpha1.PostgreSQLInstance), required, g.awsProvider, observed)
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	switch observed.GetKind() {
	case "Instance":
		return GetRDSInstanceReadyStatus(observed)
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured) map[string]*fnv1.ResourceSelector {
	switch compositeResource.GetKind() {
	case XRKindPostgreSQL:
		secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", compositeResource.GetName(), "dbadmin"))
		secretNamespace := compositeResource.GetNamespace()
		providerNamespace := g.awsProvider
		return map[string]*fnv1.ResourceSelector{
			"VPC": {
				Kind:       "VPC",
				ApiVersion: "ec2.aws.m.upbound.io/v1beta1",
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}},
				Namespace:  &providerNamespace,
			},
			"KMSKey": {
				Kind:       "Key",
				ApiVersion: "kms.aws.m.upbound.io/v1beta1",
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}},
				Namespace:  &providerNamespace,
			},
			"DBSubnetGroup": {
				Kind:       "SubnetGroup",
				ApiVersion: "rds.aws.m.upbound.io/v1beta1",
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}},
				Namespace:  &providerNamespace,
			},
			"Secret": {
				Kind:       "Secret",
				ApiVersion: "v1",
				Match:      &fnv1.ResourceSelector_MatchName{MatchName: secretName},
				Namespace:  &secretNamespace,
			},
		}
	default:
		return nil
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
	postgreSQLStatus := GetPostgreSQLStatusFromDbInstance(dbInstance)

	return runtime.DefaultUnstructuredConverter.ToUnstructured(&postgreSQLStatus)
}

func (g *GroupImpl) AddStatusConditions(compositeResource *composite.Unstructured, observed map[resource.Name]resource.ObservedComposed) {
	if compositeResource.GetKind() == XRKindPostgreSQL {
		var esIsReady bool
		for _, obs := range observed {
			if obs.Resource.GetAPIVersion() == "external-secrets.io/v1" && obs.Resource.GetKind() == "ExternalSecret" {
				esIsReady = base.GetCrossplaneReadyStatus(obs.Resource) == resource.ReadyTrue
				break
			}
		}
		if esIsReady {
			base.SetConditions(compositeResource, xpv1.Available())
		} else {
			base.SetConditions(compositeResource, xpv1.Creating())
		}
	}
}
