package apis

import "errors"

type Environment struct {
	IstioGateway string `json:"istioGateway"`
}

func (e Environment) Validate() error {
	if e.IstioGateway == "" {
		return errors.New("istioGateway is required")
	}
	return nil
}
