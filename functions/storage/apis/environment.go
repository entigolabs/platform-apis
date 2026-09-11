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
	return nil
}
