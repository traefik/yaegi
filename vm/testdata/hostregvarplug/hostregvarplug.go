// Package hostregvarplug is the expvar and net/http/pprof part of
// hostregplug. Restricted plugins may not import these two packages, so
// only the modes without restrictions load it (vm/sideeffecttest). With
// Config.Kind "expvar", New publishes an expvar variable; with "pprof", it
// only uses the routes that importing net/http/pprof adds. Two instances
// of the plugin must not collide, and the host must see none of it. Its
// handler reports what the plugin sees, one line per check.
package hostregvarplug

import (
	"context"
	"expvar"
	"fmt"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"strings"
)

type Config struct {
	Kind string
}

func CreateConfig() *Config { return &Config{} }

var hits *expvar.Int

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	switch cfg.Kind {
	case "expvar":
		hits = expvar.NewInt("hostreg_hits")
		expvar.NewFloat("hostreg_f").Set(1.5)
		expvar.NewMap("hostreg_m").Add("a", 1)
		expvar.NewString("hostreg_s").Set("x")
		expvar.Publish("hostreg_func", expvar.Func(func() any { return 3 }))
	case "pprof":
	default:
		return nil, fmt.Errorf("unknown kind %q", cfg.Kind)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve(cfg.Kind, w, r)
	}), nil
}

// serve writes one line per check of kind. The X-Host-Path header is a
// route of the host mux, which the plugin must not see.
func serve(kind string, w http.ResponseWriter, r *http.Request) {
	line := func(format string, a ...any) { fmt.Fprintf(w, format+"\n", a...) }
	line("host route: %q", pattern(r.Header.Get("X-Host-Path")))
	switch kind {
	case "expvar":
		hits.Add(1)
		line("expvar: %v %v", expvar.Get("hostreg_hits"), expvar.Get("hostreg_none") == nil)
		var names []string
		expvar.Do(func(kv expvar.KeyValue) { names = append(names, kv.Key) })
		line("expvar.Do: %s", strings.Join(names, " "))
		code, body := get("/debug/vars")
		line("/debug/vars: %d %v %v", code, strings.Contains(body, `"hostreg_hits": `), strings.Contains(body, `"hostreg_m": {"a": 1}`))
	case "pprof":
		line("pprof route: %q", strings.TrimPrefix(pattern("/debug/pprof/cmdline"), "GET "))
		code, body := get("/debug/pprof/")
		line("/debug/pprof/: %d %v", code, strings.Contains(body, "Types of profiles available"))
	}
}

func pattern(path string) string {
	_, p := http.DefaultServeMux.Handler(httptest.NewRequest("GET", path, nil))
	return p
}

// get serves path with the plugin's http.DefaultServeMux.
func get(path string) (int, string) {
	rec := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.String()
}
