package service

import (
	"fmt"
	"path"

	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/upbound/provider-aws/apis/namespaced/ecr/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/kms/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func GenerateRepositoryObject(repository v1alpha1.Repository, required map[string][]resource.Required) (map[string]runtime.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var kms kmsmv1beta1.Key
	if err = base.ExtractRequiredResource(required, apis.KMSDataKey, &kms); err != nil {
		return nil, err
	}
	encryptionType := "KMS"
	annotations := make(map[string]string)
	if repository.Spec.Path != "" || repository.Spec.Name != "" {
		annotations["crossplane.io/external-name"] = getExternalRepoName(repository)
	}
	objects := make(map[string]runtime.Object)
	region := kms.Status.AtProvider.Region
	if region == nil {
		region = kms.Spec.ForProvider.Region
	}
	if region == nil {
		return nil, fmt.Errorf("KMS key %s must have a region", kms.Name)
	}
	repo := &v1beta1.Repository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apis.RepositoryApiVersion,
			Kind:       apis.RepositoryKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repository.Name,
			Namespace: repository.Namespace,
			Labels: map[string]string{
				base.ResourceLabel:     repository.Name,
				base.ResourceKindLabel: apis.XRKindRepository,
			},
			Annotations: annotations,
		},
		Spec: v1beta1.RepositorySpec{
			InitProvider: v1beta1.RepositoryInitParameters{
				EncryptionConfiguration: []v1beta1.EncryptionConfigurationInitParameters{{
					EncryptionType: &encryptionType,
					KMSKeyRef:      &xpv2v1.NamespacedReference{Name: kms.Name, Namespace: kms.Namespace},
				}},
			},
			ForProvider: v1beta1.RepositoryParameters{
				Region:             region,
				ImageTagMutability: env.ImageTagMutability,
				Tags:               env.Tags,
			},
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpv2.ProviderConfigReference{Name: env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
		},
	}
	if env.ScanOnPush != nil {
		repo.Spec.ForProvider.ImageScanningConfiguration = &v1beta1.ImageScanningConfigurationParameters{
			ScanOnPush: env.ScanOnPush,
		}
	}
	objects[repository.Name] = repo
	return objects, nil
}

func getExternalRepoName(repository v1alpha1.Repository) string {
	name := repository.Name
	if repository.Spec.Name != "" {
		name = repository.Spec.Name
	}
	return path.Join(repository.Spec.Path, name)
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}
