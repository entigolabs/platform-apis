package apis

import (
	"errors"
	"fmt"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
)

var validImageTagMutability = base.NewSet("MUTABLE", "IMMUTABLE", "IMMUTABLE_WITH_EXCLUSION", "MUTABLE_WITH_EXCLUSION")

type Environment struct {
	AWSProvider        string             `json:"awsProvider"`
	VPC                string             `json:"vpc"`
	DataKMSKey         string             `json:"dataKMSKey"`
	ScanOnPush         *bool              `json:"scanOnPush,omitempty"`
	ImageTagMutability *string            `json:"imageTagMutability,omitempty"`
	Tags               map[string]*string `json:"tags,omitempty"`
	// LifecycleRules is the default ECR lifecycle policy for every repository in this environment.
	LifecycleRules []v1alpha1.LifecycleRule `json:"lifecycleRules,omitempty"`
}

func (e Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.VPC == "" {
		return errors.New("vpc is required")
	}
	if e.ImageTagMutability != nil && !validImageTagMutability.Contains(*e.ImageTagMutability) {
		return fmt.Errorf("imageTagMutability must be either null or %s", validImageTagMutability.String())
	}
	return v1alpha1.ValidateLifecycleRules(e.LifecycleRules)
}
