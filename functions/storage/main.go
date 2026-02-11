// Package main implements a Composition Function.
package main

import (
	"github.com/alecthomas/kong"
	"github.com/entigolabs/function-base/base"
)

type CLI struct {
	base.CLI
}

// Run this Function.
func (c *CLI) Run() error {
	return c.CLI.Run(&GroupImpl{})
}

func main() {
	cli := &CLI{}
	ctx := kong.Parse(cli, kong.Description("Entigo Storage Composition Function."))
	ctx.FatalIfErrorf(ctx.Run())
}
