// Package pluginhost calls the plugins of the measured catalog, for the tests
// and the measurement tools of yaegi v2 (compare/bench, compare/catalog). These
// plugins follow one shape: a CreateConfig function that returns a *Config,
// and a New function that returns an http.Handler (a middleware) or a value
// with Init, Provide and Stop methods (a provider). The calls of that shape
// are in package host (plugin.go there), for the hosts outside yaegi v2; this
// package gives the same names.
package pluginhost

import (
	"context"
	"net/http"

	"github.com/traefik/yaegi/v2/host"
	"github.com/traefik/yaegi/v2/vm"
)

// The limits of Options: the values of package host, where the defaults
// are.
const (
	Budget         = host.Budget
	BudgetRate     = host.BudgetRate
	BackgroundRate = host.BackgroundRate
	MaxMemory      = host.MaxMemory
	MaxStack       = host.MaxStack
)

// Options returns the options that the catalog host loads the plugins with:
// host.Options, the options that Traefik gets (AOT mode, limits,
// StaleAOTFallback, MaxStack, PanicOnFatal, StubAssembly, the options that
// accept what yaegi accepted, and no variable of the host environment).
// Every default is in package host, so what the catalog measures is what a
// host of package host gets. The host then sets Dir and UseUnsafe.
func Options() vm.Options { return host.Options() }

// The environment of a plugin: the functions of package host (env.go there).

// EnvFilter is host.EnvFilter.
type EnvFilter = host.EnvFilter

// DefaultEnv is host.DefaultEnv.
func DefaultEnv() []string { return host.DefaultEnv() }

// HostEnv is host.HostEnv.
func HostEnv() []string { return host.HostEnv() }

// SetEnv is host.SetEnv.
func SetEnv(opts *vm.Options, env []string) { host.SetEnv(opts, env) }

// The calls of a plugin: the functions of package host (plugin.go there).

// CreateConfig is host.CreateConfig.
func CreateConfig(p *vm.Plugin) (*vm.Object, error) { return host.CreateConfig(p) }

// NewHandler is host.NewHandler.
func NewHandler(ctx context.Context, p *vm.Plugin, next http.Handler, cfg *vm.Object, name string) (http.Handler, error) {
	return host.NewHandler(ctx, p, next, cfg, name)
}

// NewInstanceHandler is host.NewInstanceHandler.
func NewInstanceHandler(ctx context.Context, p *vm.Plugin, next http.Handler, cfg *vm.Object, name string) (http.Handler, *vm.Instance, error) {
	return host.NewInstanceHandler(ctx, p, next, cfg, name)
}

// NewProvider is host.NewProvider.
func NewProvider(ctx context.Context, p *vm.Plugin, cfg *vm.Object, name string) (*vm.Object, error) {
	return host.NewProvider(ctx, p, cfg, name)
}

// CallMethod is host.CallMethod.
func CallMethod(o *vm.Object, name string, args ...any) ([]any, error) {
	return host.CallMethod(o, name, args...)
}
