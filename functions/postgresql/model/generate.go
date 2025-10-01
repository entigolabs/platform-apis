// This should be run if the model structs change, to update the deep copy code.
// Generated manifests can be used as the basis for Crossplane CompositeResourceDefinitions.
//go:generate rm -rf ../crd
//go:generate go run -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen paths=./v1alpha1 object crd:crdVersions=v1,allowDangerousTypes=true output:artifacts:config=../crd

package model
