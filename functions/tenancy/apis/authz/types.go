package authz

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	APIVersion          = "authz.entigo.com/v1alpha1"
	ScopeCluster        = "Cluster"
	ReplicatedFromLabel = "authz.entigo.com/replicated-from"
	ZoneGroup           = "tenancy.entigo.com"
	ZoneKind            = "Zone"
	WorkspaceGroup      = "tenancy.kcp.io"
	WorkspaceKind       = "Workspace"
)

type Rule struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
	Scope     string   `json:"scope,omitempty"`
}

type RoleDefinition struct {
	metav1.ObjectMeta `json:"metadata"`
	Spec              RoleDefinitionSpec `json:"spec"`
}

type RoleDefinitionSpec struct {
	Rules []Rule `json:"rules"`
}

type RoleGrant struct {
	metav1.ObjectMeta `json:"metadata"`
	Spec              RoleGrantSpec `json:"spec"`
}

type RoleGrantSpec struct {
	RoleRef   string           `json:"roleRef"`
	Resources []TargetResource `json:"resources,omitempty"`
}

type TargetResource struct {
	Group string `json:"group,omitempty"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
}
