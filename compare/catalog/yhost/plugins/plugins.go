// Package plugins holds the provider interface of Traefik
// (pkg/plugins/providers.go): PP and its yaegi wrapper _PP. yaegi finds the
// wrapper of an interface by the package path of the interface type, so they
// must live in their own package, as in Traefik, not in package main.
package plugins

import (
	"encoding/json"
	"reflect"
)

// PP is the interface of a provider plugin.
type PP interface {
	Init() error
	Provide(cfgChan chan<- json.Marshaler) error
	Stop() error
}

type _PP struct {
	IValue   any
	WInit    func() error
	WProvide func(cfgChan chan<- json.Marshaler) error
	WStop    func() error
}

func (p _PP) Init() error                                 { return p.WInit() }
func (p _PP) Provide(cfgChan chan<- json.Marshaler) error { return p.WProvide(cfgChan) }
func (p _PP) Stop() error                                 { return p.WStop() }

// Path is the import path of this package in the interpreter.
const Path = "github.com/traefik/yaegi/v2/compare/catalog/yhost/plugins"

// Symbols exports PP to the interpreter. Traefik adds them when it creates
// the interpreter, before the import of the plugin (builder.go).
func Symbols() map[string]map[string]reflect.Value {
	return map[string]map[string]reflect.Value{
		Path + "/plugins": {
			"PP":  reflect.ValueOf((*PP)(nil)),
			"_PP": reflect.ValueOf((*_PP)(nil)),
		},
	}
}
