package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// newProvider builds a provider plugin with pluginhost.NewProvider.
func (e *vmEngine) newProvider(ctx context.Context, cfg any, name string) (providerPlugin, error) {
	o, err := pluginhost.NewProvider(ctx, e.p, cfg.(*vm.Object), name)
	if err != nil {
		return nil, err
	}
	return vmProvider{o}, nil
}

// vmProvider calls the methods of the provider value with pluginhost.CallMethod.
type vmProvider struct{ o *vm.Object }

func (p vmProvider) call(name string, args ...any) error {
	res, err := pluginhost.CallMethod(p.o, name, args...)
	if err != nil {
		return err
	}
	if len(res) != 1 {
		return fmt.Errorf("%s returns %d values", name, len(res))
	}
	if res[0] == nil {
		return nil
	}
	e, ok := res[0].(error)
	if !ok {
		return errors.New("result of " + name + " is not an error")
	}
	return e
}

func (p vmProvider) Init() error { return p.call("Init") }

func (p vmProvider) Provide(cfgChan chan<- json.Marshaler) error { return p.call("Provide", cfgChan) }

func (p vmProvider) Stop() error { return p.call("Stop") }
