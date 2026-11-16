// Package flightplug starts a zero trace.FlightRecorder without naming
// Start, through an interface of its own and through reflect. WriteTo
// would give the trace of every goroutine of the host: each path must be
// refused when it runs.
package flightplug

import (
	"context"
	"net/http"
	"reflect"
	"runtime/trace"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type starter interface{ Start() error }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var fr any = new(trace.FlightRecorder)
		switch r.URL.Path {
		case "/interface":
			fr.(starter).Start()
		case "/reflect":
			reflect.ValueOf(fr).MethodByName("Start").Call(nil)
		}
		if fr.(*trace.FlightRecorder).Enabled() {
			w.Write([]byte("started"))
		}
	}), nil
}
