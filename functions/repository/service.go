package main

import (
	"fmt"

	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/model/v1alpha1"
	"github.com/upbound/provider-aws/apis/cluster/ecr/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type environment struct {
	provider string
	region   string
}

func GenerateRepositoryObject(repository v1alpha1.Repository, required map[string][]resource.Required) (map[string]runtime.Object, error) {
	if err := checkRepositoryConflict(repository, required); err != nil {
		return nil, err
	}

	env, err := getEnvironment(required)
	if err != nil {
		return nil, err
	}

	objects := make(map[string]runtime.Object)
	// TODO Add org tag?
	tags := make(map[string]*string)
	for _, tag := range repository.Spec.Tags {
		tags[tag.Key] = &tag.Value
	}
	repo := &v1beta2.Repository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: RepositoryApiVersion,
			Kind:       RepositoryKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repository.Name,
			Namespace: repository.Namespace,
			Labels: map[string]string{
				"region":               env.region,
				base.ResourceLabel:     repository.Name,
				base.ResourceKindLabel: XRKindRepository,
			},
		},
		Spec: v1beta2.RepositorySpec{
			ForProvider: v1beta2.RepositoryParameters{
				Region: &env.region,
				Tags:   tags,
				ImageScanningConfiguration: &v1beta2.ImageScanningConfigurationParameters{
					ScanOnPush: repository.Spec.ImageScanningConfiguration.ScanOnPush,
				},
			},
			ResourceSpec: xpv2v1.ResourceSpec{
				ProviderConfigReference: &xpv2.Reference{
					Name: env.provider,
				},
			},
		},
	}
	if repository.Spec.ImageTagMutability != nil {
		mutability := string(*repository.Spec.ImageTagMutability)
		repo.Spec.ForProvider.ImageTagMutability = &mutability
	}
	objects[repository.Name] = repo
	return objects, nil
}

func getEnvironment(required map[string][]resource.Required) (environment, error) {
	data, err := base.GetEnvironmentData(base.EnvironmentKey, required[base.EnvironmentKey])
	if err != nil {
		return environment{}, err
	}
	provider, err := base.GetRequiredDataString(data, "provider")
	if err != nil {
		return environment{}, err
	}
	region, err := base.GetRequiredDataString(data, "region")
	if err != nil {
		return environment{}, err
	}
	return environment{
		provider: provider,
		region:   region,
	}, nil
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
