package main

import (
	"fmt"

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
	if err := checkRepositoryConflict(repository, required); err != nil {
		return nil, err
	}

	env, err := getEnvironment(required[base.EnvironmentKey])
	if err != nil {
		return nil, err
	}

	var kms kmsmv1beta1.Key
	if err = base.ExtractRequiredResource(required, KMSDataKey, &kms); err != nil {
		return nil, err
	}
	encryptionType := "KMS"

	objects := make(map[string]runtime.Object)
	repo := &v1beta1.Repository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: RepositoryApiVersion,
			Kind:       RepositoryKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repository.Name,
			Namespace: repository.Namespace,
			Labels: map[string]string{
				"region":               env.AWSRegion,
				base.ResourceLabel:     repository.Name,
				base.ResourceKindLabel: XRKindRepository,
			},
		},
		Spec: v1beta1.RepositorySpec{
			ForProvider: v1beta1.RepositoryParameters{
				Region:             &env.AWSRegion,
				ImageTagMutability: env.ImageTagMutability,
				EncryptionConfiguration: []v1beta1.EncryptionConfigurationParameters{{
					EncryptionType: &encryptionType,
					KMSKeyRef:      &xpv2v1.NamespacedReference{Name: kms.Name, Namespace: kms.Namespace},
				}},
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

func getEnvironment(resources []resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, resources, &env)
	return env, err
}

func checkRepositoryConflict(repository v1alpha1.Repository, required map[string][]resource.Required) error {
	repositories := required[RequiredRepositoryKey]
	if len(repositories) == 0 {
		return nil
	}
	conflictRepository := repositories[0].Resource
	if conflictRepository.GetNamespace() != repository.GetNamespace() {
		return fmt.Errorf("repository %s already exists in namespace %s", repository.Name, conflictRepository.GetNamespace())
	}
	annotations := conflictRepository.GetAnnotations()
	actualAnnotationValue, annotationFound := annotations["crossplane.io/composition-resource-name"]

	if !annotationFound || actualAnnotationValue != repository.GetName() {
		return fmt.Errorf("repository %s object already exists", repository.GetName())
	}
	return nil
}
