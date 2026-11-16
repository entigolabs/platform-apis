package main

import (
	"github.com/alecthomas/kong"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
)

type CLI struct {
	base.CLI
	CloudProvider     apis.CloudProvider `help:"Cluster cloud provider type" env:"CLOUD_PROVIDER" default:"aws" enum:"aws,oracle"`
	AuthzEnabled      bool               `help:"Build zone access from RoleGrants instead of roleMapping groups" env:"AUTHZ_ENABLED"`
	AuthzOrganization string             `help:"Organization of the cluster, used when AUTHZ_ENABLED is true" env:"AUTHZ_ORGANIZATION"`
	AuthzWorkspace    string             `help:"Workspace of the cluster" env:"AUTHZ_WORKSPACE"`
}

func (c *CLI) Run() error {
	return c.CLI.Run(&GroupImpl{
		cloudProvider: c.CloudProvider,
		authz:         apis.Authz{Grants: c.AuthzEnabled, Organization: c.AuthzOrganization, Workspace: c.AuthzWorkspace},
	})
}

func main() {
	cli := &CLI{}
	ctx := kong.Parse(cli, kong.Description("Entigo Tenancy Composition Function."))
	ctx.FatalIfErrorf(ctx.Run())
}
