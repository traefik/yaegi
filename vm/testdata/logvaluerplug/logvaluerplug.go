// Package logvaluerplug has types whose LogValue method hides a secret
// (slog.LogValuer). Logs writes, with a logger of its own, a value of each
// type; Values gives them to the host, which logs them with its own
// logger. The secret must never be written: the slog of the host and the
// native slog of a plugin without the AOT copy of log/slog must call
// LogValue.
package logvaluerplug

import (
	"bytes"
	"errors"
	"log/slog"
)

// Token has only LogValue.
type Token struct{ Secret string }

func (t Token) LogValue() slog.Value { return slog.StringValue("token-hidden") }

// Named also has String.
type Named struct{ Secret string }

func (n Named) String() string       { return "named" }
func (n Named) LogValue() slog.Value { return slog.StringValue("named-hidden") }

// ErrAuth is an error that wraps ErrBase and has LogValue.
type ErrAuth struct{ Secret string }

var ErrBase = errors.New("base")

func (e *ErrAuth) Error() string        { return "auth failed" }
func (e *ErrAuth) Unwrap() error        { return ErrBase }
func (e *ErrAuth) LogValue() slog.Value { return slog.StringValue("auth-hidden") }

// Group has a LogValue that is a group.
type Group struct{ Secret string }

func (g Group) LogValue() slog.Value {
	return slog.GroupValue(slog.String("user", "bob"), slog.Int("n", 1))
}

func noTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

// Logs logs the values with a logger of the plugin, with the function and
// with a func value of it.
func Logs() string {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: noTime}))
	err := error(&ErrAuth{"pw-err"})
	l.Info("direct", "t", Token{"pw-token"}, "n", Named{"pw-named"}, "e", err, "g", Group{"pw-group"})
	info := l.Info
	info("value", slog.Any("t", Token{"pw-token"}), slog.Any("e", err))
	return buf.String() + "is " + boolString(errors.Is(err, ErrBase))
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Values gives the values to the host.
func Values() []any {
	return []any{Token{"pw-token"}, Named{"pw-named"}, &ErrAuth{"pw-err"}, Group{"pw-group"}}
}
