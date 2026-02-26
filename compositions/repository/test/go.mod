module test

go 1.25.5

require (
	github.com/crossplane/crossplane-runtime/v2 v2.2.0
	github.com/crossplane/crossplane/v2 v2.2.0
	github.com/entigolabs/platform-apis/test/common/crossplane v0.0.0
	github.com/spf13/afero v1.15.0
	k8s.io/apimachinery v0.35.1
)

replace github.com/entigolabs/platform-apis/test/common/crossplane => ../../../test/common/crossplane
