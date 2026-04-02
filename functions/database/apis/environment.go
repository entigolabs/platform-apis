package apis

import (
	"errors"
)

type Environment struct {
	AWSProvider                  string             `json:"awsProvider"`
	DataKMSKey                   string             `json:"dataKMSKey"`
	ConfigKMSKey                 string             `json:"configKMSKey"`
	VPC                          string             `json:"vpc"`
	SubnetGroup                  string             `json:"subnetGroup"`
	ElasticacheSubnetGroup       string             `json:"elasticacheSubnetGroup"`
	EsClusterSecretStore         string             `json:"esClusterSecretStore"`
	PostgresBackupBeforeDeletion *bool              `json:"postgresBackupBeforeDeletion"`
	ValkeyBackupBeforeDeletion   *bool              `json:"valkeyBackupBeforeDeletion"`
	BackupRetentionPeriod        *float64           `json:"backupRetentionPeriod"`
	Tags                         map[string]*string `json:"tags,omitempty"`
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
	if e.PostgresBackupBeforeDeletion == nil {
		defaultTrue := true
		e.PostgresBackupBeforeDeletion = &defaultTrue
	}
	if e.ValkeyBackupBeforeDeletion == nil {
		defaultTrue := true
		e.ValkeyBackupBeforeDeletion = &defaultTrue
	}
	if e.BackupRetentionPeriod == nil {
		return errors.New("backupRetentionPeriod is required")
	}
	return nil
}
