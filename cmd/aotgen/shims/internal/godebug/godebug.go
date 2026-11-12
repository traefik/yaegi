// Package godebug is the aotgen shim of internal/godebug: the original links
// into the runtime (go:linkname) and cannot be copied out of the standard
// library. This version reads $GODEBUG once and has no metrics.
package godebug

import (
	"os"
	"strings"
	"sync"
)

// Setting is a single GODEBUG setting.
type Setting struct {
	name  string
	once  sync.Once
	value string
}

// New returns the setting named name (a leading '#' marks it undocumented).
func New(name string) *Setting { return &Setting{name: name} }

// Name returns the name of the setting.
func (s *Setting) Name() string {
	if s.name != "" && s.name[0] == '#' {
		return s.name[1:]
	}
	return s.name
}

// Undocumented reports whether this is an undocumented setting.
func (s *Setting) Undocumented() bool { return s.name != "" && s.name[0] == '#' }

// String returns a printable form for the setting: name=value.
func (s *Setting) String() string { return s.Name() + "=" + s.Value() }

// IncNonDefault is a no-op (no runtime metrics outside the standard library).
func (s *Setting) IncNonDefault() {}

// Value returns the current value of the setting ("" when unset).
func (s *Setting) Value() string {
	s.once.Do(func() {
		for _, kv := range strings.Split(os.Getenv("GODEBUG"), ",") {
			if k, v, ok := strings.Cut(kv, "="); ok && k == s.Name() {
				s.value = v
			}
		}
	})
	return s.value
}
