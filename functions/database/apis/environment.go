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
	MariaDBBackupBeforeDeletion  *bool              `json:"mariaDBBackupBeforeDeletion"`
	ValkeyBackupBeforeDeletion   *bool              `json:"valkeyBackupBeforeDeletion"`
	BackupRetentionPeriod        *float64           `json:"backupRetentionPeriod"`
	Tags                         map[string]*string `json:"tags,omitempty"`
}

func (e *Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
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
		e.PostgresBackupBeforeDeletion = new(true)
	}
	if e.MariaDBBackupBeforeDeletion == nil {
		e.MariaDBBackupBeforeDeletion = new(true)
	}
	if e.ValkeyBackupBeforeDeletion == nil {
		e.ValkeyBackupBeforeDeletion = new(true)
	}
	if e.BackupRetentionPeriod == nil {
		return errors.New("backupRetentionPeriod is required")
	}
	return nil
}
