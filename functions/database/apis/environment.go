package apis

import (
	"errors"
)

type Environment struct {
	AWSProvider  string `json:"awsProvider"`
	DataKMSKey   string `json:"dataKMSKey"`
	ConfigKMSKey string `json:"configKMSKey"`
	VPC          string `json:"vpc"`
	SubnetGroup  string `json:"subnetGroup"`
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
	if e.VPC == "" {
		return errors.New("vpc is required")
	}
	if e.SubnetGroup == "" {
		return errors.New("subnetGroup is required")
	}
	return nil
}
