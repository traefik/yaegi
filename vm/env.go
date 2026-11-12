package vm

// The environment of a restricted plugin. yaegi in restricted mode gives
// each interpreter its own copy of the environment (its host usually passes
// os.Environ()): os.Setenv and friends change that copy, never the
// environment of the host process. A Machine with an environment (SetEnv)
// does the same: the os functions that read or write the environment are
// replaced by functions over its copy, for direct calls and function values.

import (
	"os"
	"reflect"
	"strings"
	"sync"
)

// vmEnv is the environment copy of a Machine.
type vmEnv struct {
	mu   sync.RWMutex
	vars map[string]string
}

// addOverride records that the native function key ("pkg.Name", no receiver)
// is replaced by fn for this machine (nativeFunc reads m.overrides).
func (m *Machine) addOverride(key string, fn any) {
	if m.overrides == nil {
		m.overrides = map[string]reflect.Value{}
	}
	m.overrides[key] = reflect.ValueOf(fn)
}

// SetEnv gives the machine its own environment, initialized from env
// ("key=value" entries, as os.Environ returns). It must be called before
// the program runs.
func (m *Machine) SetEnv(env []string) {
	e := &vmEnv{vars: map[string]string{}}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			e.vars[k] = v
		}
	}
	getenv := func(key string) string {
		e.mu.RLock()
		defer e.mu.RUnlock()
		return e.vars[key]
	}
	m.addOverride("os.Getenv", getenv)
	m.addOverride("os.LookupEnv", func(key string) (string, bool) {
		e.mu.RLock()
		defer e.mu.RUnlock()
		v, ok := e.vars[key]
		return v, ok
	})
	m.addOverride("os.Setenv", func(key, value string) error {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return &os.SyscallError{Syscall: "setenv", Err: errInvalidEnv}
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.vars[key] = value
		return nil
	})
	m.addOverride("os.Unsetenv", func(key string) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.vars, key)
		return nil
	})
	m.addOverride("os.Clearenv", func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.vars = map[string]string{}
	})
	m.addOverride("os.Environ", e.environ)
	m.addOverride("os.ExpandEnv", func(s string) string { return os.Expand(s, getenv) })
	m.env = e
}

type envError string

func (e envError) Error() string { return string(e) }

const errInvalidEnv = envError("invalid argument")
