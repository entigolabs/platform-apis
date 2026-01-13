package main

import (
	"github.com/alecthomas/kong"
	"github.com/entigolabs/function-base/base"
)

type CLI struct {
	base.CLI
}

func (c *CLI) Run() error {
	return c.CLI.Run(&GroupImpl{})
}

func main() {
	cli := &CLI{}
	ctx := kong.Parse(cli, kong.Description("Entigo Tenancy Composition Function."))
	ctx.FatalIfErrorf(ctx.Run())
}
