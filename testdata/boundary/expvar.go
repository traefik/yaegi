// Native boundary of expvar: program expvar.Var types given to expvar
// (docs/BOUNDARY.md).
package main

import (
	"expvar"
	"fmt"
	"log"
	"net/http/httptest"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// vv is a Var (value receiver).
type vv struct{ n int }

func (v vv) String() string { return fmt.Sprintf(`{"vv":%d}`, v.n) }

// pv is a Var (pointer receiver).
type pv struct{ s string }

func (p *pv) String() string { return fmt.Sprintf("%q", "pv:"+p.s) }

// gv is a generic Var.
type gv[T any] struct{ v T }

func (g gv[T]) String() string { return fmt.Sprintf(`"gv:%v"`, g.v) }

// num is a named int that is a Var.
type num int

func (n num) String() string { return fmt.Sprint(int(n) * 2) }

// lst is a named slice that is a Var.
type lst []string

func (l lst) String() string { return `["` + strings.Join(l, `","`) + `"]` }

// prefix is the start of every name published by this program.
const prefix = "boundary_expvar_"

func main() {
	vars := map[string]expvar.Var{
		"vv": vv{1}, "pv": &pv{"p"}, "gv": gv[int]{2}, "gvs": gv[string]{"s"}, "num": num(3), "lst": lst{"a", "b"},
		"func": expvar.Func(func() any { return map[string]int{"f": 4} }),
	}
	// row: expvar (*Map).Do,(*Map).Get,(*Map).String assert x.(expvar.Var)
	probe("Map", func() {
		m := new(expvar.Map).Init()
		for k, v := range vars {
			m.Set(k, v)
		}
		var iface expvar.Var = vv{5}
		m.Set("iface", iface)
		out("String %s", m.String())
		m.Do(func(kv expvar.KeyValue) { out("Do %s %T %s", kv.Key, kv.Value, kv.Value) })
		for _, k := range []string{"vv", "pv", "gv", "num", "lst", "iface", "missing"} {
			v := m.Get(k)
			if v == nil {
				out("Get %s nil", k)
				continue
			}
			out("Get %s %T %s", k, v, v.String())
		}
		pm := expvar.NewMap(prefix + "map")
		pm.Set("in", vv{6})
		pm.Add("count", 2)
		out("NewMap %s", pm.String())
	})
	// row: expvar Do,Get,Handler assert x.(expvar.Var)
	probe("Published", func() {
		for _, k := range []string{"vv", "pv", "gv", "gvs", "num", "lst", "func"} {
			expvar.Publish(prefix+k, vars[k])
		}
		for _, k := range []string{"vv", "pv", "gv", "num", "lst", "func", "missing"} {
			v := expvar.Get(prefix + k)
			if v == nil {
				out("Get %s nil", k)
				continue
			}
			out("Get %s %T %s", k, v, v.String())
		}
		expvar.Do(func(kv expvar.KeyValue) {
			if strings.HasPrefix(kv.Key, prefix) {
				out("Do %s %T %s", kv.Key, kv.Value, kv.Value)
			}
		})
		rec := httptest.NewRecorder()
		expvar.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/debug/vars", nil))
		out("Handler %d %s", rec.Code, rec.Header().Get("Content-Type"))
		for _, line := range strings.Split(rec.Body.String(), "\n") {
			if strings.HasPrefix(line, `"`+prefix) {
				out("Handler %s", strings.TrimSuffix(line, ","))
			}
		}
		// publishing a name twice panics (log.Panicln)
		var logged strings.Builder
		log.SetOutput(&logged)
		log.SetFlags(0)
		func() {
			defer func() { r := recover(); out("Publish twice: %T %q", r, r) }()
			expvar.Publish(prefix+"vv", vv{9})
		}()
		out("logged %q", logged.String())
	})
}
