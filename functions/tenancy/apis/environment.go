package apis

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CloudProviderAWS    = "aws"
	CloudProviderOracle = "oracle"
)

type Environment struct {
	AppProject                     AppProject         `json:"appProject"`
	ArgoCDNamespace                string             `json:"argoCDNamespace"`
	AWSProvider                    string             `json:"awsProvider"`
	CloudProvider                  string             `json:"cloudProvider,omitempty"`
	Cluster                        string             `json:"cluster"`
	CreateAppsNamespace            bool               `json:"createAppsNamespace"`
	ComputeSubnetType              string             `json:"computeSubnetType"`
	DataKMSAlias                   string             `json:"dataKMSAlias"`
	DataKMSKey                     string             `json:"dataKMSKey,omitempty"`
	GranularEgress                 bool               `json:"granularEgress,omitempty"`
	GranularEgressExclude          []string           `json:"granularEgressExclude,omitempty"`
	GranularNamespaceNetworkPolicy bool               `json:"granularNamespaceNetworkPolicy,omitempty"`
	OracleProvider                 string             `json:"oracleProvider,omitempty"`
	PodSecurity                    string             `json:"podSecurity"`
	ReferenceNodePools             []string           `json:"referenceNodePools,omitempty"`
	RoleMapping                    []RoleMapping      `json:"roleMapping,omitempty"`
	SecurityGroup                  string             `json:"securityGroup"`
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

func (e Environment) GetCloudProvider() string {
	if e.CloudProvider == "" {
		return CloudProviderAWS
	}
	return e.CloudProvider
}

func (e Environment) GetReferenceNodePools() []string {
	if len(e.ReferenceNodePools) == 0 {
		return []string{"tools", "main", "mon"}
	}
	return e.ReferenceNodePools
}

func (e Environment) Validate() error {
	switch e.GetCloudProvider() {
	case CloudProviderAWS:
		if e.AWSProvider == "" {
			return errors.New("awsProvider is required")
		}
		if e.VPC == "" {
			return errors.New("vpc is required")
		}
	case CloudProviderOracle:
		if e.OracleProvider == "" {
			return errors.New("oracleProvider is required")
		}
	default:
		return fmt.Errorf("unsupported cloudProvider %q", e.CloudProvider)
	}
	if e.ArgoCDNamespace == "" {
		return errors.New("argoCDNamespace is required")
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
	if e.PodSecurity == "" {
		return errors.New("podSecurity is required")
	}
	return nil
}
