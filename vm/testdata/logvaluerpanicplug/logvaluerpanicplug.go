// Package logvaluerpanicplug has types whose LogValue method panics or
// never returns. Values gives them to the host, which logs them with its
// own log/slog: slog must recover the panic and write "LogValue panicked",
// as with go run, and the goroutine of the host must go on.
package logvaluerpanicplug

import "log/slog"

// Bad panics in LogValue.
type Bad struct{}

func (Bad) LogValue() slog.Value { panic("bad-value") }

// Spin loops in LogValue until the budget of the call runs out.
type Spin struct{}

func (Spin) LogValue() slog.Value {
	n := 0
	for {
		n++
	}
}

// Values gives the values to the host.
func Values() []any { return []any{Bad{}, Spin{}} }
