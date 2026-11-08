package main

import (
	"github.com/alecthomas/kong"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
)

type CLI struct {
	base.CLI
	CloudProvider     apis.CloudProvider `help:"Cluster cloud provider type" env:"CLOUD_PROVIDER" default:"aws" enum:"aws,oracle"`
	AuthzOrganization string             `help:"Organization of the cluster, enables AppProject roles from RoleGrants" env:"AUTHZ_ORGANIZATION"`
	AuthzWorkspace    string             `help:"Workspace of the cluster" env:"AUTHZ_WORKSPACE"`
}

func (c *CLI) Run() error {
	return c.CLI.Run(&GroupImpl{
		cloudProvider: c.CloudProvider,
		authz:         apis.Authz{Organization: c.AuthzOrganization, Workspace: c.AuthzWorkspace},
	})
}

func main() {
	cli := &CLI{}
	ctx := kong.Parse(cli, kong.Description("Entigo Tenancy Composition Function."))
	ctx.FatalIfErrorf(ctx.Run())
}
