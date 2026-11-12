package main

import (
	"context"
	"fmt"
	"reflect"

	"github.com/traefik/yaegi/v2/compare/catalog/yhost/plugins"
)

// newProvider evaluates the wrapper package of Traefik around New
// (pkg/plugins/providers.go, newProvider) and calls it. The only change is
// the import path of the PP interface (see package plugins).
func (e *yaegiEngine) newProvider(ctx context.Context, cfg any, name string) (providerPlugin, error) {
	_, err := e.i.Eval(`package wrapper

import (
	"context"

	` + e.basePkg + ` "` + e.imp + `"
	"` + plugins.Path + `"
)

func NewWrapper(ctx context.Context, config *` + e.basePkg + `.Config, name string) (plugins.PP, error) {
	p, err := ` + e.basePkg + `.New(ctx, config, name)
	var pv plugins.PP = p
	return pv, err
}
`)
	if err != nil {
		return nil, fmt.Errorf("failed to eval wrapper: %w", err)
	}
	fnNew, err := e.i.Eval("wrapper.NewWrapper")
	if err != nil {
		return nil, fmt.Errorf("failed to eval New: %w", err)
	}
	res := fnNew.Call([]reflect.Value{reflect.ValueOf(ctx), cfg.(reflect.Value), reflect.ValueOf(name)})
	if len(res) > 1 && res[1].Interface() != nil {
		return nil, res[1].Interface().(error)
	}
	p, ok := res[0].Interface().(plugins.PP)
	if !ok {
		return nil, fmt.Errorf("invalid provider type: %T", res[0].Interface())
	}
	return p, nil
}
