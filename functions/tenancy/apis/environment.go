package apis

import (
	"errors"
)

type Environment struct {
	AppProject            AppProject         `json:"appProject"`
	ArgoCDNamespace       string             `json:"argoCDNamespace"`
	AWSProvider           string             `json:"awsProvider"`
	Cluster               string             `json:"cluster"`
	DataKMSAlias          string             `json:"dataKMSAlias"`
	SecurityGroup         string             `json:"securityGroup"`
	ComputeSubnetType     string             `json:"computeSubnetType"`
	ServiceSubnetType     string             `json:"serviceSubnetType"`
	PublicSubnetType      string             `json:"publicSubnetType"`
	ControlSubnetType     string             `json:"controlSubnetType"`
	Tags                  map[string]*string `json:"tags,omitempty"`
	VPC                   string             `json:"vpc"`
	GranularEgress        bool               `json:"granularEgress,omitempty"`
	GranularEgressExclude []string           `json:"granularEgressExclude,omitempty"`
}

type AppProject struct {
	MaintainerGroups []string `json:"maintainerGroups,omitempty"`
	ObserverGroups   []string `json:"observerGroups,omitempty"`
}

func (e Environment) Validate() error {
	if e.AWSProvider == "" {
		return errors.New("awsProvider is required")
	}
	if e.ArgoCDNamespace == "" {
		return errors.New("argoCDNamespace is required")
	}
	if e.VPC == "" {
		return errors.New("vpc is required")
	}
	if e.DataKMSAlias == "" {
		return errors.New("dataKMSAlias is required")
	}
	if e.SecurityGroup == "" {
		return errors.New("securityGroup is required")
	}
	if e.Cluster == "" {
		return errors.New("cluster is required")
	}
	if e.ComputeSubnetType == "" {
		return errors.New("computeSubnetType is required")
	}
	if e.ServiceSubnetType == "" {
		return errors.New("serviceSubnetType is required")
	}
	if e.PublicSubnetType == "" {
		return errors.New("publicSubnetType is required")
	}
	if e.ControlSubnetType == "" {
		return errors.New("controlSubnetType is required")
	}
	return nil
}
