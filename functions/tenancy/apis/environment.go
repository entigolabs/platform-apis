package apis

import (
	"errors"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CloudProvider string

const (
	CloudProviderAWS CloudProvider = "aws"
	CloudProviderOCI CloudProvider = "oracle"
)

type Environment struct {
	AppProject                     AppProject         `json:"appProject"`
	ArgoCDNamespace                string             `json:"argoCDNamespace"`
	AWSProvider                    string             `json:"awsProvider"`
	Cluster                        string             `json:"cluster"`
	CreateAppsNamespace            bool               `json:"createAppsNamespace"`
	ComputeSubnetType              string             `json:"computeSubnetType"`
	DataKMSAlias                   string             `json:"dataKMSAlias"`
	DataKMSKey                     string             `json:"dataKMSKey,omitempty"`
	GranularEgress                 bool               `json:"granularEgress,omitempty"`
	GranularEgressExclude          []string           `json:"granularEgressExclude,omitempty"`
	GranularNamespaceNetworkPolicy bool               `json:"granularNamespaceNetworkPolicy,omitempty"`
	NamespaceRole                  NamespaceRole      `json:"namespaceRole,omitempty"`
	OCIProvider                    string             `json:"oracleProvider,omitempty"`
	PodSecurity                    string             `json:"podSecurity"`
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

// NamespaceRole configures the Role bound to maintainers and contributors in Zone Namespaces.
type NamespaceRole struct {
	Rules []rbacv1.PolicyRule `json:"rules,omitempty"`
}

type RoleMapping struct {
	RoleRef string   `json:"roleRef"`
	Groups  []string `json:"groups"`
}

func (e Environment) ValidateFor(provider CloudProvider) error {
	// stop instead of emptying every Zone Namespace Role, e.g. when an upgrade applies the function first
	if e.NamespaceRole.Rules == nil {
		return errors.New("namespaceRole.rules is required")
	}
	switch provider {
	case CloudProviderOCI:
		if e.OCIProvider == "" {
			return errors.New("oracleProvider is required")
		}
	default:
		if e.AWSProvider == "" {
			return errors.New("awsProvider is required")
		}
		if e.VPC == "" {
			return errors.New("vpc is required")
		}
	}
	return nil
}

func (e Environment) Validate() error {
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
