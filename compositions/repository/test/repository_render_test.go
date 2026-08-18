package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	composition     = "../apis/repository-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/artifact"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"

	repositoryResource          = "../examples/repository-with-name-path.yaml"
	repositoryLifecycleResource = "../examples/repository-with-lifecycle.yaml"
)

func TestRepositoryCrossplaneRender(t *testing.T) {
	t.Logf("Starting artifact function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, repositoryResource, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Repository", 2)

	t.Log("Validating artifact.entigo.com Repository fields")
	crossplane.AssertFieldValues(t, resources, "Repository", "artifact.entigo.com/v1alpha1", map[string]string{
		"metadata.name": "repository-example",
		"spec.name":     "example",
		"spec.path":     "helm/dev",
	})

	t.Log("Validating ecr.aws.m.upbound.io Repository fields")
	crossplane.AssertFieldValues(t, resources, "Repository", "ecr.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.annotations.crossplane\\.io/external-name": "helm/dev/example",
		"metadata.name":                                             "repository-example",
		"metadata.ownerReferences.0.apiVersion":                     "artifact.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                           "Repository",
		"metadata.ownerReferences.0.name":                           "repository-example",
		"spec.forProvider.encryptionConfiguration.0.encryptionType": "KMS",
		"spec.forProvider.encryptionConfiguration.0.kmsKey":         "arn:aws:kms:eu-north-1:012345678901:key/mrk-0",
		"spec.forProvider.region":                                   "eu-north-1",
	})

	t.Log("Mocking observed resources")
	mockedRepo := crossplane.MockByKind(t, resources, "Repository", "ecr.aws.m.upbound.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, mockedRepo)

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, repositoryResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting artifact.entigo.com Repository Ready Status")
	crossplane.AssertResourceReady(t, resources, "Repository", "artifact.entigo.com/v1alpha1")
}

func TestRepositoryLifecyclePolicyCrossplaneRender(t *testing.T) {
	t.Logf("Starting artifact function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, repositoryLifecycleResource, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting the lifecycle policy is not created before the repository is ready")
	crossplane.AssertResourceCount(t, resources, "LifecyclePolicy", 0)

	t.Log("Mocking observed resources")
	mockedRepo := crossplane.MockByKind(t, resources, "Repository", "ecr.aws.m.upbound.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, mockedRepo)

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, repositoryLifecycleResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting the lifecycle policy is created once the repository is ready")
	crossplane.AssertResourceCount(t, resources, "LifecyclePolicy", 1)

	t.Log("Validating ecr.aws.m.upbound.io LifecyclePolicy fields")
	crossplane.AssertFieldValues(t, resources, "LifecyclePolicy", "ecr.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":               "repository-lifecycle-example",
		"spec.forProvider.region":     "eu-north-1",
		"spec.forProvider.repository": "repository-lifecycle-example",
		"spec.forProvider.policy":     lifecyclePolicyDocument,
	})
}

const lifecyclePolicyDocument = `{"rules":[{"rulePriority":1,"description":"Keep 10 latest images tagged with prefix develop","selection":{"tagStatus":"tagged","tagPrefixList":["develop"],"countType":"imageCountMoreThan","countNumber":10},"action":{"type":"expire"}},{"rulePriority":2,"description":"Keep 10 latest images matching tag pattern *-cloud","selection":{"tagStatus":"tagged","tagPatternList":["*-cloud"],"countType":"imageCountMoreThan","countNumber":10},"action":{"type":"expire"}},{"rulePriority":3,"description":"Expire untagged images older than 7 days","selection":{"tagStatus":"untagged","countType":"sinceImagePushed","countUnit":"days","countNumber":7},"action":{"type":"expire"}},{"rulePriority":4,"description":"Expire images older than 90 days","selection":{"tagStatus":"any","countType":"sinceImagePushed","countUnit":"days","countNumber":90},"action":{"type":"expire"}}]}`
