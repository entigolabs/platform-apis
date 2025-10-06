// Package main implements a Composition Function.
package main

import (
	"github.com/alecthomas/kong"
	"github.com/entigolabs/function-base/base"
)

type CLI struct {
	base.CLI
	AWSProvider string `help:"Crossplane AWS provider name" env:"AWS_PROVIDER"`
	AWSRegion   string `help:"AWS region for the provider" env:"AWS_REGION" default:"eu-north-1"`
}

// Run this Function.
func (c *CLI) Run() error {
	service := NewGroupImpl(c.AWSProvider, c.AWSRegion)
	return c.CLI.Run(service)
}

func main() {
	cli := &CLI{}
	ctx := kong.Parse(cli, kong.Description("Entigo Repository Composition Function."))
	// Kong required seems to trigger before env vars are parsed, validate manually
	if cli.AWSProvider == "" {
		ctx.Fatalf("AWSProvider must be set")
	} else if cli.AWSRegion == "" {
		ctx.Fatalf("AWSRegion must be set")
	}
	ctx.FatalIfErrorf(ctx.Run())
}
