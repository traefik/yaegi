package pluginhost

// The environment of a plugin. A restricted plugin of yaegi v2 has its own copy
// of the environment (vm.Options.Env, vm/env.go): os.Getenv, LookupEnv,
// Environ and ExpandEnv read the copy, os.Setenv, Unsetenv and Clearenv
// change the copy, never the environment of the host process or of another
// plugin. The host chooses what goes into the copy:
//
//   - HostEnv: every variable of the host, as Traefik gives a yaegi plugin
//     (interp.Options{Env: os.Environ()}). Plugins that read their settings
//     or secrets from the environment work as under yaegi.
//   - EnvFilter.Env: only the host variables that the host lists, by exact
//     name or by prefix (as the settings.envs of the Wasm plugins of
//     Traefik). For a host that does not want to give every secret of the
//     process to third-party code.
//   - an empty slice (DefaultEnv, what Options sets): no variable at all.
//
// None of them is a security boundary: the plugin runs in the host process
// and shares its memory (docs/SANDBOX.md).
// They stop an accidental read or a careless log of the environment.

import (
	"os"
	"strings"

	"github.com/traefik/yaegi/v2/vm"
)

// DefaultEnv returns the environment that Options gives a plugin: empty, not
// nil (a nil vm.Options.Env would copy the whole host environment). The
// host passes the environment itself, as an option, as Traefik passes
// interp.Options.Env to yaegi: SetEnv with HostEnv or an EnvFilter
// (docs/SANDBOX.md, "The environment of a plugin"). It is the one place of
// that default.
func DefaultEnv() []string {
	return []string{}
}

// HostEnv returns a copy of the environment of the host process, for
// vm.Options.Env: what Traefik gives a plugin under yaegi. The copy is
// taken when HostEnv runs; later changes of the host environment are not
// seen, as under yaegi.
func HostEnv() []string {
	return os.Environ()
}

// EnvFilter selects the variables of the host environment that a plugin
// sees. A host builds one per plugin. The zero EnvFilter selects nothing.
type EnvFilter struct {
	// Names are the exact names of the variables the plugin sees.
	Names []string
	// Prefixes are name prefixes: a variable whose name starts with one of
	// them is seen ("OIDC_" selects OIDC_CLIENT_ID and OIDC_SECRET). An
	// empty prefix is ignored: an empty entry in the settings of a plugin
	// must not give it every secret of the host (HostEnv does that).
	Prefixes []string
}

// Env returns the variables of the host environment that f selects, for
// vm.Options.Env. It never returns nil: a nil vm.Options.Env means the
// whole host environment, an empty slice means no variable.
func (f EnvFilter) Env() []string {
	return f.Filter(os.Environ())
}

// Filter returns the entries of env ("key=value", as os.Environ returns
// them) whose name f selects, in the order of env. It never returns nil.
func (f EnvFilter) Filter(env []string) []string {
	out := []string{}
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if ok && f.selects(k) {
			out = append(out, kv)
		}
	}
	return out
}

// selects reports whether the variable named k passes f.
func (f EnvFilter) selects(k string) bool {
	for _, n := range f.Names {
		if k == n {
			return true
		}
	}
	for _, p := range f.Prefixes {
		if p != "" && strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// SetEnv sets the environment of the plugin that opts loads: opts.Env, and
// opts.Sandbox.Env when opts has a Sandbox (a Sandbox ignores opts.Env). A
// nil env is an empty environment here, not the host environment: use
// HostEnv for that.
func SetEnv(opts *vm.Options, env []string) {
	if env == nil {
		env = []string{}
	}
	opts.Env = env
	if opts.Sandbox != nil {
		opts.Sandbox.Env = env
	}
}
