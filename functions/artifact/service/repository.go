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

func GenerateRepositoryObject(repository v1alpha1.Repository, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var kms kmsmv1beta1.Key
	if err = base.ExtractRequiredResource(required, apis.KMSDataKey, &kms); err != nil {
		return nil, err
	}
	encryptionType := "KMS"
	var annotations map[string]string
	if repository.Spec.Path != "" || repository.Spec.Name != "" {
		annotations = map[string]string{"crossplane.io/external-name": getExternalRepoName(repository)}
	}
	objects := make(map[string]runtime.Object)
	region := kms.Status.AtProvider.Region
	if region == nil {
		region = kms.Spec.ForProvider.Region
	}
	if region == nil {
		return nil, fmt.Errorf("KMS key %s must have a region", kms.Name)
	}
	encryption := v1beta1.EncryptionConfigurationParameters{
		EncryptionType: &encryptionType,
		KMSKeyRef:      &xpv2v1.NamespacedReference{Name: kms.Name, Namespace: kms.Namespace},
	}
	kmsArn := getKMSKeyARN(repository.Name, observed)
	if kmsArn != nil {
		encryption.KMSKey = kmsArn
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
			InitProvider: v1beta1.RepositoryInitParameters{},
			ForProvider: v1beta1.RepositoryParameters{
				Region:                  region,
				ImageTagMutability:      env.ImageTagMutability,
				Tags:                    env.Tags,
				EncryptionConfiguration: []v1beta1.EncryptionConfigurationParameters{encryption},
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

func getKMSKeyARN(name string, observed map[resource.Name]resource.ObservedComposed) *string {
	observedResource, ok := observed[resource.Name(name)]
	if !ok {
		return nil
	}
	var observedRepo v1beta1.Repository
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observedResource.Resource.Object, &observedRepo); err != nil {
		return nil
	}
	return observedRepo.Spec.ForProvider.EncryptionConfiguration[0].KMSKey
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
