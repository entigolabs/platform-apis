package apis

import "errors"

type Environment struct {
	AWSProvider        string  `json:"awsProvider"`
	AWSRegion          string  `json:"awsRegion"`
	DataKMSKey         string  `json:"dataKMSKey"`
	ScanOnPush         *bool   `json:"scanOnPush,omitempty"`
	ImageTagMutability *string `json:"imageTagMutability,omitempty"`
}

func (e *Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.AWSRegion == "" {
		return errors.New("awsRegion is required")
	}
	if e.DataKMSKey == "" {
		return errors.New("dataKMSKey is required")
	}
	if e.ImageTagMutability != nil && *e.ImageTagMutability != "MUTABLE" && *e.ImageTagMutability != "IMMUTABLE" {
		return errors.New("imageTagMutability must be either 'MUTABLE' or 'IMMUTABLE' if specified")
	}
	return nil
}
