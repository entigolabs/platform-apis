package apis

import (
	"errors"
)

type Environment struct {
	AWSProvider    string             `json:"awsProvider"`
	ConfigKMSKey   string             `json:"configKMSKey"`
	KafkaNamespace string             `json:"kafkaNamespace"`
	Tags           map[string]*string `json:"tags,omitempty"`
}

func (e *Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.ConfigKMSKey == "" {
		return errors.New("configKMSKey is required")
	}
	if e.KafkaNamespace == "" {
		return errors.New("kafkaNamespace is required")
	}
	return nil
}
