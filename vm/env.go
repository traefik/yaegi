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
	// touched holds the names that the plugin set or unset itself, and
	// cleared is true after Clearenv: a read of such a name that finds
	// nothing is not a variable the host did not pass (missing).
	touched map[string]bool
	cleared bool
	// partial is true when the copy left out a variable of the host process
	// (SetEnv): a read of the whole environment then reports
	// MissingEnvEnviron.
	partial bool

	// missing is Options.OnMissingEnv, or nil. reported holds the names
	// already given to it (under repMu), and names all of them in order
	// (for Machine.MissingEnv). full is true once the names reached
	// maxMissingEnvNames and missing got MissingEnvMore.
	missing  func(name string)
	repMu    sync.Mutex
	reported map[string]bool
	names    []string
	full     bool
}

// The plugin chooses the names it reads, and the names are kept outside
// Options.MaxMemory, in the memory of the host: a plugin that reads a new
// long name on each request must not make the host memory and logs grow
// without end. A machine keeps (and reports) at most maxMissingEnvNames
// names, each cut to maxMissingEnvNameLen bytes.
const (
	maxMissingEnvNames   = 256
	maxMissingEnvNameLen = 128
)

// MissingEnvMore is the name that Options.OnMissingEnv gets once, after the
// first maxMissingEnvNames names: the plugin read more missing variables,
// and they are not reported. It is empty: a real name is never empty.
const MissingEnvMore = ""

// MissingEnvEnviron is the name that Options.OnMissingEnv gets once when the
// plugin reads its whole environment (os.Environ, syscall.Environ,
// unix.Environ, (*exec.Cmd).Environ) while the host did not pass every
// variable of its process (an EnvFilter, the empty default). Such a plugin
// (jwt-middleware puts os.Environ() into its templates) reads no name, so
// no other name tells the host that it misses variables. A real name has no
// parentheses in practice.
const MissingEnvEnviron = "(os.Environ)"

// lookup reads the variable key of e. When it is not there, and the plugin
// did not set, unset or clear it itself, it is a variable that the host did
// not pass: e records its name (cut to maxMissingEnvNameLen), once, and
// calls missing with it (never with a value), up to maxMissingEnvNames
// names.
func (e *vmEnv) lookup(key string) (string, bool) {
	e.mu.RLock()
	v, ok := e.vars[key]
	own := e.cleared || e.touched[key]
	e.mu.RUnlock()
	if !ok && !own && key != "" {
		e.report(key)
	}
	return v, ok
}

// report records the missing name key and calls missing with it the first
// time (see lookup).
func (e *vmEnv) report(key string) {
	if len(key) > maxMissingEnvNameLen {
		key = key[:maxMissingEnvNameLen] + "..."
	} else {
		key = strings.Clone(key) // not a part of a larger string of the plugin
	}
	e.repMu.Lock()
	if e.reported[key] {
		e.repMu.Unlock()
		return
	}
	if len(e.names) >= maxMissingEnvNames {
		more := !e.full
		e.full = true
		e.repMu.Unlock()
		if more && e.missing != nil {
			e.missing(MissingEnvMore)
		}
		return
	}
	if e.reported == nil {
		e.reported = map[string]bool{}
	}
	e.reported[key] = true
	e.names = append(e.names, key)
	e.repMu.Unlock()
	if e.missing != nil {
		e.missing(key)
	}
}

// MissingEnv returns the names of the variables that the plugin read and
// did not find in its environment copy, though it never set, unset or
// cleared them itself: variables the host did not pass (Options.Env, an
// EnvFilter). Each name comes once, in the order of the first read, cut to
// 128 bytes, and there are at most 256 names (see Options.OnMissingEnv). It is
// nil for a machine without an environment copy (SetEnv). The values are
// never kept.
func (m *Machine) MissingEnv() []string {
	e := m.env
	if e == nil {
		return nil
	}
	e.repMu.Lock()
	defer e.repMu.Unlock()
	return append([]string(nil), e.names...)
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
	e := &vmEnv{vars: map[string]string{}, touched: map[string]bool{}, missing: m.onMissingEnv}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			e.vars[k] = v
		}
	}
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && k != "" {
			if _, in := e.vars[k]; !in {
				e.partial = true
				break
			}
		}
	}
	getenv := func(key string) string {
		v, _ := e.lookup(key)
		return v
	}
	m.addOverride("os.Getenv", getenv)
	m.addOverride("os.LookupEnv", e.lookup)
	m.addOverride("os.Setenv", func(key, value string) error {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return &os.SyscallError{Syscall: "setenv", Err: errInvalidEnv}
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.vars[key] = value
		e.touched[key] = true
		return nil
	})
	m.addOverride("os.Unsetenv", func(key string) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.vars, key)
		e.touched[key] = true
		return nil
	})
	m.addOverride("os.Clearenv", func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.vars = map[string]string{}
		e.cleared = true
	})
	m.addOverride("os.Environ", e.environ)
	m.addOverride("os.ExpandEnv", func(s string) string { return os.Expand(s, getenv) })
	m.env = e
}

type envError string

func (e envError) Error() string { return string(e) }

const errInvalidEnv = envError("invalid argument")
