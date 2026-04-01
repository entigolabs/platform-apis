package base

import "github.com/crossplane/function-sdk-go"

type CLI struct {
	Debug bool `short:"d" help:"Emit debug logs in addition to info logs."`

	Network     string `help:"Network on which to listen for gRPC connections." default:"tcp"`
	Address     string `help:"Address at which to listen for gRPC connections." default:":9443"`
	TLSCertsDir string `help:"Directory containing server certs (tls.key, tls.crt) and the CA used to verify client certificates (ca.crt)" env:"TLS_SERVER_CERTS_DIR"`
	Insecure    bool   `help:"Run without mTLS credentials. If you supply this flag --tls-server-certs-dir will be ignored."`
	Workspace   string `help:"Entigo Platform Workspace" env:"WORKSPACE"`
}

func (c *CLI) Run(service GroupService) error {
	log, err := function.NewLogger(c.Debug)
	if err != nil {
		return err
	}
	service.SetLogger(log)
	return function.Serve(NewFunction(log, service, c.Workspace),
		function.Listen(c.Network, c.Address),
		function.MTLSCertificates(c.TLSCertsDir),
		function.Insecure(c.Insecure))
}
