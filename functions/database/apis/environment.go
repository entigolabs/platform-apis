package apis

import (
	"errors"
)

type Environment struct {
	AWSProvider          string             `json:"awsProvider"`
	DataKMSKey           string             `json:"dataKMSKey"`
	ConfigKMSKey         string             `json:"configKMSKey"`
	VPC                  string             `json:"vpc"`
	SubnetGroup          string             `json:"subnetGroup"`
	EsClusterSecretStore string             `json:"esClusterSecretStore"`
	Tags                 map[string]*string `json:"tags,omitempty"`
	BackupBeforeDeletion *bool              `json:"backupBeforeDeletion"`
}

func (e *Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.DataKMSKey == "" {
		return errors.New("dataKMSKey is required")
	}
	if e.ConfigKMSKey == "" {
		return errors.New("configKMSKey is required")
	}
	if e.VPC == "" {
		return errors.New("vpc is required")
	}
	if e.SubnetGroup == "" {
		return errors.New("subnetGroup is required")
	}
	if e.ElasticacheSubnetGroup == "" {
		return errors.New("elasticacheSubnetGroup is required")
	}
	if e.EsClusterSecretStore == "" {
		return errors.New("esClusterSecretStore is required")
	}
	if e.BackupBeforeDeletion == nil {
		defaultTrue := true
		e.BackupBeforeDeletion = &defaultTrue
	}
	return nil
}
