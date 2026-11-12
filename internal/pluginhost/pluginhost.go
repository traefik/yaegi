// Package pluginhost calls the plugins of the measured catalog, for the tests
// and the measurement tools of yaegi v2 (compare/bench, compare/catalog). These
// plugins follow one shape: a CreateConfig function that returns a *Config,
// and a New function that returns an http.Handler (a middleware) or a value
// with Init, Provide and Stop methods (a provider). The vm package knows
// nothing of this shape: it calls functions and methods by name.
package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/traefik/yaegi/v2/host"
	"github.com/traefik/yaegi/v2/vm"
)

// Budget, BackgroundRate and MaxMemory are the limits of Options
// (package host, where the defaults are).
const (
	Budget         = host.Budget
	BackgroundRate = host.BackgroundRate
	MaxMemory      = host.MaxMemory
)

// MaxStack is the Options.MaxStack of Options: the stack of one goroutine
// of the plugin (registers, frame-local variables, deferred calls), past
// which it stops with vm.ErrStackOverflow (256 MiB). The vm package keeps
// 1 GiB, as gc, for a host without other limits; a host that loads plugins
// it does not trust also sets MaxMemory, and 256 MiB stops a runaway
// recursion of one goroutine long before it fills a quarter of the quota.
// The cost: a valid program with deep frames can stop with
// ErrStackOverflow here and run under the vm default. Each element of a
// frame-local array is a 24-byte Value, so 3000 frames with a local
// [4096]byte (12 MB of stack under gc) need about 290 MiB
// (TestDefaultMaxStackDeep runs it with the vm default).
const MaxStack = 256 << 20

// Options returns the options that the catalog host loads the plugins with:
// host.Options, the public options of a host that loads plugins it does not
// trust (AOT mode, limits, PanicOnFatal, StubAssembly and the options that
// accept what yaegi accepted), with no variable of the host environment
// (DefaultEnv; the host passes the environment as an option, as Traefik
// passes interp.Options.Env to yaegi: HostEnv, or an EnvFilter per plugin,
// with SetEnv: see env.go). Package host imports
// github.com/traefik/yaegi/v2/aot/all. It also sets StaleAOTFallback: a host
// built with a newer Go patch release than the AOT copies, which changed
// the sources of a copied package, loads the plugins in default mode
// instead of refusing all of them; the host logs Plugin.Notes. It sets
// MaxStack (256 MiB, above). host.Check
// still refuses stale copies: the catalog host loads with vm.Load. The host
// then sets Dir and UseUnsafe.
func Options() vm.Options {
	o := host.Options()
	o.Env = DefaultEnv()
	o.StaleAOTFallback = true
	o.MaxStack = MaxStack
	return o
}

// CreateConfig calls the CreateConfig function of p.
func CreateConfig(p *vm.Plugin) (*vm.Object, error) {
	res, err := p.Call(context.Background(), "CreateConfig")
	if err != nil {
		return nil, err
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("the CreateConfig function returns %d results, 1 expected", len(res))
	}
	return res[0], nil
}

// NewHandler calls New(ctx, next, cfg, name) (http.Handler, error) of p.
func NewHandler(ctx context.Context, p *vm.Plugin, next http.Handler, cfg *vm.Object, name string) (http.Handler, error) {
	res, err := p.Call(ctx, "New", ctx, next, cfg, name)
	if err != nil {
		return nil, err
	}
	if len(res) != 2 {
		return nil, fmt.Errorf("the New function returns %d results, 2 expected", len(res))
	}
	if err := resultErr(res[1]); err != nil {
		return nil, err
	}
	v, err := res[0].Interface()
	if err != nil {
		return nil, err
	}
	h, ok := v.(http.Handler)
	if !ok {
		return nil, fmt.Errorf("the New function returns %T, an http.Handler is expected", v)
	}
	return h, nil
}

// NewProvider calls New(ctx, cfg, name) (T, error) of p, and returns the T
// value, whose methods CallMethod calls.
func NewProvider(ctx context.Context, p *vm.Plugin, cfg *vm.Object, name string) (*vm.Object, error) {
	res, err := p.Call(ctx, "New", ctx, cfg, name)
	if err != nil {
		return nil, err
	}
	if len(res) != 2 {
		return nil, fmt.Errorf("the New function returns %d results, 2 expected", len(res))
	}
	if err := resultErr(res[1]); err != nil {
		return nil, err
	}
	return res[0], nil
}

// CallMethod calls the method name of o and returns its results as native
// values (a nil error is a nil any).
func CallMethod(o *vm.Object, name string, args ...any) ([]any, error) {
	res, err := o.Call(context.Background(), name, args...)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(res))
	for i, r := range res {
		if out[i], err = r.Interface(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resultErr returns the error that the error result r holds.
func resultErr(r *vm.Object) error {
	v, err := r.Interface()
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	e, ok := v.(error)
	if !ok {
		return errors.New("the error result is not an error")
	}
	return e
}
