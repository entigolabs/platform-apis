package apis

import (
	"errors"
	"fmt"

	"github.com/entigolabs/function-base/base"
)

var validImageTagMutability = base.NewSet("MUTABLE", "IMMUTABLE", "IMMUTABLE_WITH_EXCLUSION", "MUTABLE_WITH_EXCLUSION")

type Environment struct {
	AWSProvider        string  `json:"awsProvider"`
	DataKMSKey         string  `json:"dataKMSKey"`
	ScanOnPush         *bool   `json:"scanOnPush,omitempty"`
	ImageTagMutability *string `json:"imageTagMutability,omitempty"`
}

func (e Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.DataKMSKey == "" {
		return errors.New("dataKMSKey is required")
	}
	if e.ImageTagMutability != nil && !validImageTagMutability.Contains(*e.ImageTagMutability) {
		return fmt.Errorf("imageTagMutability must be either null or %s", validImageTagMutability.String())
	}
	return nil
}
