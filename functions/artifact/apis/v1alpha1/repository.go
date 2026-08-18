// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=artifact.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	"errors"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

const (
	// MaxLifecycleRules is the number of rules AWS allows in a single ECR lifecycle policy.
	MaxLifecycleRules = 25
	// MaxExpireAfterDays is the largest age AWS accepts for a sinceImagePushed rule.
	MaxExpireAfterDays = 365
)

// Repository generates OCI repository resources
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RepositorySpec   `json:"spec,omitempty"`
	Status            RepositoryStatus `json:"status,omitempty"`
}

type RepositorySpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	Path string `json:"path,omitempty"`
	// LifecycleRules replaces the default rules from the environment config when set.
	// +kubebuilder:validation:Optional
	LifecycleRules []LifecycleRule `json:"lifecycleRules,omitempty"`
}

// LifecycleRule expires images from a repository. Exactly one selector (tagPrefixes, tagPatterns or
// untagged) and exactly one retention bound (keepCount or expireAfterDays) must be set. A rule
// without a selector matches every image and must therefore be the last rule.
type LifecycleRule struct {
	// +kubebuilder:validation:Optional
	TagPrefixes []string `json:"tagPrefixes,omitempty"`
	// +kubebuilder:validation:Optional
	TagPatterns []string `json:"tagPatterns,omitempty"`
	// +kubebuilder:validation:Optional
	Untagged bool `json:"untagged,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	KeepCount *int `json:"keepCount,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=365
	ExpireAfterDays *int `json:"expireAfterDays,omitempty"`
}

// MatchesAllImages reports whether the rule has no selector and therefore applies to every image.
func (r *LifecycleRule) MatchesAllImages() bool {
	return len(r.TagPrefixes) == 0 && len(r.TagPatterns) == 0 && !r.Untagged
}

func ValidateLifecycleRules(rules []LifecycleRule) error {
	if len(rules) > MaxLifecycleRules {
		return fmt.Errorf("lifecycleRules must not contain more than %d rules, got %d", MaxLifecycleRules, len(rules))
	}
	for i, rule := range rules {
		if err := rule.validate(); err != nil {
			return fmt.Errorf("lifecycleRules[%d]: %w", i, err)
		}
		if rule.MatchesAllImages() && i != len(rules)-1 {
			return fmt.Errorf("lifecycleRules[%d]: a rule without a selector matches all images and must be the last rule", i)
		}
	}
	return nil
}

func (r *LifecycleRule) validate() error {
	selectors := 0
	if len(r.TagPrefixes) > 0 {
		selectors++
	}
	if len(r.TagPatterns) > 0 {
		selectors++
	}
	if r.Untagged {
		selectors++
	}
	if selectors > 1 {
		return errors.New("only one of tagPrefixes, tagPatterns or untagged may be set")
	}
	if err := validateTagList("tagPrefixes", r.TagPrefixes); err != nil {
		return err
	}
	if err := validateTagList("tagPatterns", r.TagPatterns); err != nil {
		return err
	}
	if (r.KeepCount == nil) == (r.ExpireAfterDays == nil) {
		return errors.New("exactly one of keepCount or expireAfterDays must be set")
	}
	if r.KeepCount != nil && *r.KeepCount < 1 {
		return fmt.Errorf("keepCount must be at least 1, got %d", *r.KeepCount)
	}
	if r.ExpireAfterDays != nil && (*r.ExpireAfterDays < 1 || *r.ExpireAfterDays > MaxExpireAfterDays) {
		return fmt.Errorf("expireAfterDays must be between 1 and %d, got %d", MaxExpireAfterDays, *r.ExpireAfterDays)
	}
	return nil
}

func validateTagList(field string, values []string) error {
	if slices.Contains(values, "") {
		return fmt.Errorf("%s must not contain empty values", field)
	}
	return nil
}

type RepositoryStatus struct {
	RepositoryUri string `json:"repositoryUri,omitempty"`
}
