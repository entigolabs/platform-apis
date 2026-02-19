package apis

import (
	"errors"
)

type Environment struct {
	AWSProvider  string             `json:"awsProvider"`
	DataKMSKey   string             `json:"dataKMSKey"`
	ConfigKMSKey string             `json:"configKMSKey"`
	Tags         map[string]*string `json:"tags,omitempty"`
}

func (e Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.DataKMSKey == "" {
		return errors.New("dataKMSKey is required")
	}
	if e.ConfigKMSKey == "" {
		return errors.New("configKMSKey is required")
	}
	return nil
}
