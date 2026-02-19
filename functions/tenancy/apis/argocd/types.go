// Package argocd contains the AppProject types extracted from
// github.com/argoproj/argo-cd to avoid importing the full argo-cd module
// and its heavy transitive dependency tree (gitops-engine, k8s.io/kubernetes, etc.).
// +kubebuilder:object:generate=true
package argocd

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
type AppProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`
	Spec              AppProjectSpec   `json:"spec" protobuf:"bytes,2,opt,name=spec"`
	Status            AppProjectStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

type AppProjectStatus struct {
	JWTTokensByRole map[string]JWTTokens `json:"jwtTokensByRole,omitempty" protobuf:"bytes,1,opt,name=jwtTokensByRole"`
}

type JWTTokens struct {
	Items []JWTToken `json:"items,omitempty" protobuf:"bytes,1,rep,name=items"`
}

type JWTToken struct {
	IssuedAt  int64  `json:"iat" protobuf:"int64,1,opt,name=iat"`
	ExpiresAt int64  `json:"exp,omitempty" protobuf:"int64,2,opt,name=exp"`
	ID        string `json:"id,omitempty" protobuf:"bytes,3,opt,name=id"`
}

type AppProjectSpec struct {
	SourceRepos                []string                         `json:"sourceRepos,omitempty" protobuf:"bytes,1,name=sourceRepos"`
	Destinations               []ApplicationDestination         `json:"destinations,omitempty" protobuf:"bytes,2,name=destination"`
	Description                string                           `json:"description,omitempty" protobuf:"bytes,3,opt,name=description"`
	Roles                      []ProjectRole                    `json:"roles,omitempty" protobuf:"bytes,4,rep,name=roles"`
	ClusterResourceWhitelist   []ClusterResourceRestrictionItem `json:"clusterResourceWhitelist,omitempty" protobuf:"bytes,5,opt,name=clusterResourceWhitelist"`
	NamespaceResourceBlacklist []metav1.GroupKind               `json:"namespaceResourceBlacklist,omitempty" protobuf:"bytes,6,opt,name=namespaceResourceBlacklist"`
	NamespaceResourceWhitelist []metav1.GroupKind               `json:"namespaceResourceWhitelist,omitempty" protobuf:"bytes,9,opt,name=namespaceResourceWhitelist"`
	ClusterResourceBlacklist   []ClusterResourceRestrictionItem `json:"clusterResourceBlacklist,omitempty" protobuf:"bytes,11,opt,name=clusterResourceBlacklist"`
	SourceNamespaces           []string                         `json:"sourceNamespaces,omitempty" protobuf:"bytes,12,opt,name=sourceNamespaces"`
}

type ClusterResourceRestrictionItem struct {
	Group string `json:"group" protobuf:"bytes,1,opt,name=group"`
	Kind  string `json:"kind" protobuf:"bytes,2,opt,name=kind"`
	Name  string `json:"name,omitempty" protobuf:"bytes,3,opt,name=name"`
}

type ApplicationDestination struct {
	Server    string `json:"server,omitempty" protobuf:"bytes,1,opt,name=server"`
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
	Name      string `json:"name,omitempty" protobuf:"bytes,3,opt,name=name"`
}

type ProjectRole struct {
	Name        string     `json:"name" protobuf:"bytes,1,opt,name=name"`
	Description string     `json:"description,omitempty" protobuf:"bytes,2,opt,name=description"`
	Policies    []string   `json:"policies,omitempty" protobuf:"bytes,3,rep,name=policies"`
	JWTTokens   []JWTToken `json:"jwtTokens,omitempty" protobuf:"bytes,4,rep,name=jwtTokens"`
	Groups      []string   `json:"groups,omitempty" protobuf:"bytes,5,rep,name=groups"`
}
