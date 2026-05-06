package apis

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Environment struct {
	AppProject                     AppProject         `json:"appProject"`
	ArgoCDNamespace                string             `json:"argoCDNamespace"`
	AWSProvider                    string             `json:"awsProvider"`
	Cluster                        string             `json:"cluster"`
	ComputeSubnetType              string             `json:"computeSubnetType"`
	ControlSubnetType              string             `json:"controlSubnetType"`
	DataKMSAlias                   string             `json:"dataKMSAlias"`
	GranularEgress                 bool               `json:"granularEgress,omitempty"`
	GranularEgressExclude          []string           `json:"granularEgressExclude,omitempty"`
	GranularNamespaceNetworkPolicy bool               `json:"granularNamespaceNetworkPolicy,omitempty"`
	PodSecurity                    string             `json:"podSecurity"`
	PublicSubnetType               string             `json:"publicSubnetType"`
	RoleMapping                    []RoleMapping      `json:"roleMapping,omitempty"`
	SecurityGroup                  string             `json:"securityGroup"`
	ServiceSubnetType              string             `json:"serviceSubnetType"`
	Tags                           map[string]*string `json:"tags,omitempty"`
	VPC                            string             `json:"vpc"`
	Workspace                      string             `json:"workspace,omitempty"`
}

type AppProject struct {
	NamespaceResourceBlacklist []metav1.GroupKind `json:"namespaceResourceBlacklist,omitempty"`
	NamespaceResourceWhitelist []metav1.GroupKind `json:"namespaceResourceWhitelist,omitempty"`
	SourceRepos                []string           `json:"sourceRepos,omitempty"`
}

type RoleMapping struct {
	RoleRef string   `json:"roleRef"`
	Groups  []string `json:"groups"`
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
	if e.PodSecurity == "" {
		return errors.New("podSecurity is required")
	}
	return nil
}
