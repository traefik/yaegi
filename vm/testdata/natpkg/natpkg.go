// Package natpkg is a package of the host: the tests of natpkg_test.go
// compile it in and register its table with RegisterNativePackage, as
// cmd/aotgen -native does, and the plugin natpkgplug uses it in
// ModeNative.
package natpkg

import (
	"errors"
	"strings"
)

// Sink has an Error method that is not the one of error (logr.LogSink).
type Sink interface {
	Info(msg string)
	Error(err error, msg string)
}

// Use calls the methods of s.
func Use(s Sink) {
	s.Info("start")
	s.Error(errors.New("boom"), "failed")
}

// SinkWrapper is the wrapper of Sink, in the format of the stdsyms
// wrappers.
type SinkWrapper struct {
	IValue any
	WError func(err error, msg string)
	WInfo  func(msg string)
}

func (w SinkWrapper) Error(err error, msg string) { w.WError(err, msg) }
func (w SinkWrapper) Info(msg string)             { w.WInfo(msg) }

// State holds values of an unexported type (otel/trace.TraceState holds
// []member).
type State struct{ list []member }

type member struct{ key, value string }

func (m member) String() string { return m.key + "=" + m.value }

// NewState returns a State of the key value pairs kv.
func NewState(kv ...string) State {
	var s State
	for i := 0; i+1 < len(kv); i += 2 {
		s.list = append(s.list, member{kv[i], kv[i+1]})
	}
	return s
}

func (s State) String() string {
	parts := make([]string, len(s.list))
	for i, m := range s.list {
		parts[i] = m.String()
	}
	return strings.Join(parts, ",")
}
