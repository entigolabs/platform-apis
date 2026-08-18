package service

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/upbound/provider-aws/v2/apis/namespaced/ecr/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/kms/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LifecyclePolicyResourceName = "lifecycle-policy"
	LifecyclePolicyKind         = "LifecyclePolicy"

	tagStatusTagged   = "tagged"
	tagStatusUntagged = "untagged"
	tagStatusAny      = "any"

	countTypeImageCountMoreThan = "imageCountMoreThan"
	countTypeSinceImagePushed   = "sinceImagePushed"

	countUnitDays = "days"

	// maxDescriptionLength is the limit AWS puts on a rule description.
	maxDescriptionLength = 256
)

type lifecyclePolicy struct {
	Rules []lifecyclePolicyRule `json:"rules"`
}

type lifecyclePolicyRule struct {
	RulePriority int                         `json:"rulePriority"`
	Description  string                      `json:"description"`
	Selection    lifecyclePolicyRuleSelector `json:"selection"`
	Action       lifecyclePolicyRuleAction   `json:"action"`
}

type lifecyclePolicyRuleSelector struct {
	TagStatus      string   `json:"tagStatus"`
	TagPrefixList  []string `json:"tagPrefixList,omitempty"`
	TagPatternList []string `json:"tagPatternList,omitempty"`
	CountType      string   `json:"countType"`
	CountUnit      string   `json:"countUnit,omitempty"`
	CountNumber    int      `json:"countNumber"`
}

type lifecyclePolicyRuleAction struct {
	Type string `json:"type"`
}

func GenerateRepositoryObject(repository v1alpha1.Repository, required map[string][]resource.Required) (map[string]client.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var kms kmsmv1beta1.Key
	if err = base.ExtractRequiredResource(required, apis.KMSDataKey, &kms); err != nil {
		return nil, err
	}
	if kms.Status.AtProvider.Arn == nil {
		return nil, fmt.Errorf("KMS key %s ARN is not available", kms.Name)
	}
	encryptionType := "KMS"
	var annotations map[string]string
	if repository.Spec.Path != "" || repository.Spec.Name != "" {
		annotations = map[string]string{"crossplane.io/external-name": getExternalRepoName(repository)}
	}
	objects := make(map[string]client.Object)
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
			ForProvider: v1beta1.RepositoryParameters{
				Region:             region,
				ImageTagMutability: env.ImageTagMutability,
				Tags:               env.Tags,
				EncryptionConfiguration: []v1beta1.EncryptionConfigurationParameters{{
					EncryptionType: &encryptionType,
					KMSKey:         kms.Status.AtProvider.Arn,
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

	rules := repository.Spec.LifecycleRules
	if len(rules) == 0 {
		rules = env.LifecycleRules
	}
	if len(rules) > 0 {
		policy, err := BuildLifecyclePolicy(rules)
		if err != nil {
			return nil, err
		}
		objects[LifecyclePolicyResourceName] = newLifecyclePolicy(repository, env, region, policy)
	}
	return objects, nil
}

func newLifecyclePolicy(repository v1alpha1.Repository, env apis.Environment, region *string, policy string) *v1beta1.LifecyclePolicy {
	repoName := getExternalRepoName(repository)
	return &v1beta1.LifecyclePolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apis.RepositoryApiVersion,
			Kind:       LifecyclePolicyKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repository.Name,
			Namespace: repository.Namespace,
			Labels: map[string]string{
				base.ResourceLabel:     repository.Name,
				base.ResourceKindLabel: apis.XRKindRepository,
			},
		},
		Spec: v1beta1.LifecyclePolicySpec{
			ForProvider: v1beta1.LifecyclePolicyParameters{
				Region:     region,
				Repository: &repoName,
				Policy:     &policy,
			},
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpv2.ProviderConfigReference{Name: env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
		},
	}
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

func BuildLifecyclePolicy(rules []v1alpha1.LifecycleRule) (string, error) {
	if err := v1alpha1.ValidateLifecycleRules(rules); err != nil {
		return "", err
	}
	policy := lifecyclePolicy{Rules: make([]lifecyclePolicyRule, 0, len(rules))}
	for i, rule := range rules {
		policy.Rules = append(policy.Rules, lifecyclePolicyRule{
			RulePriority: i + 1,
			Description:  ruleDescription(rule),
			Selection:    ruleSelector(rule),
			Action:       lifecyclePolicyRuleAction{Type: "expire"},
		})
	}
	document, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("cannot marshal lifecycle policy: %w", err)
	}
	return string(document), nil
}

func ruleSelector(rule v1alpha1.LifecycleRule) lifecyclePolicyRuleSelector {
	selector := lifecyclePolicyRuleSelector{TagStatus: tagStatusAny}
	switch {
	case len(rule.TagPrefixes) > 0:
		selector.TagStatus = tagStatusTagged
		selector.TagPrefixList = rule.TagPrefixes
	case len(rule.TagPatterns) > 0:
		selector.TagStatus = tagStatusTagged
		selector.TagPatternList = rule.TagPatterns
	case rule.Untagged:
		selector.TagStatus = tagStatusUntagged
	}
	if rule.KeepCount != nil {
		selector.CountType = countTypeImageCountMoreThan
		selector.CountNumber = *rule.KeepCount
		return selector
	}
	selector.CountType = countTypeSinceImagePushed
	selector.CountUnit = countUnitDays
	selector.CountNumber = *rule.ExpireAfterDays
	return selector
}

func ruleDescription(rule v1alpha1.LifecycleRule) string {
	var description string
	if rule.KeepCount != nil {
		description = fmt.Sprintf("Keep %d latest %s", *rule.KeepCount, describeImages(rule))
	} else {
		description = fmt.Sprintf("Expire %s older than %d days", describeImages(rule), *rule.ExpireAfterDays)
	}
	if len(description) > maxDescriptionLength {
		return description[:maxDescriptionLength]
	}
	return description
}

func describeImages(rule v1alpha1.LifecycleRule) string {
	switch {
	case len(rule.TagPrefixes) > 0:
		return fmt.Sprintf("images tagged with prefix %s", strings.Join(rule.TagPrefixes, ", "))
	case len(rule.TagPatterns) > 0:
		return fmt.Sprintf("images matching tag pattern %s", strings.Join(rule.TagPatterns, ", "))
	case rule.Untagged:
		return "untagged images"
	default:
		return "images"
	}
}
