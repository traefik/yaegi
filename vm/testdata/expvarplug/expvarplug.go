// Package expvarplug imports expvar alone, as prometheus/client_golang
// does: a restricted plugin may. It gets a table of its
// own that starts empty: it sees no cmdline and no memstats of the host,
// in Get, Do and /debug/vars. Two instances publish the same names without
// a panic; a name published twice in one instance panics in the plugin, as
// in Go. Its handler reports what the plugin sees, one line per check.
package expvarplug

import (
	"context"
	"expvar"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	hits := expvar.NewInt("expvarplug_hits")
	expvar.NewMap("expvarplug_m").Add("a", 1)
	expvar.Publish("expvarplug_func", expvar.Func(func() any { return 3 }))
	dup := func() (r any) {
		defer func() { r = recover() }()
		expvar.NewInt("expvarplug_hits")
		return nil
	}()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		line := func(format string, a ...any) { fmt.Fprintf(w, format+"\n", a...) }
		hits.Add(1)
		line("duplicate: %v", dup)
		line("get: %v %v %v %v", expvar.Get("expvarplug_hits"), expvar.Get("cmdline") == nil, expvar.Get("memstats") == nil, expvar.Get("expvarplug_func"))
		var names []string
		expvar.Do(func(kv expvar.KeyValue) { names = append(names, kv.Key) })
		line("do: %s", strings.Join(names, " "))
		rec := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest("GET", "/debug/vars", nil))
		body := rec.Body.String()
		line("/debug/vars: %d %v %v %v", rec.Code, strings.Contains(body, `"expvarplug_m": {"a": 1}`),
			strings.Contains(body, "cmdline"), strings.Contains(body, "memstats"))
	}), nil
}
