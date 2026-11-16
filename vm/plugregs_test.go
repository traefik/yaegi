package vm

import (
	"context"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// regsPlugin writes a plugin that registers its type T with gob (with
// Register and RegisterName) and a pprof profile in its init, as libraries
// do (gorilla/sessions registers its types, a pool library its profile).
// tag tells two plugins apart: their types and names are the same.
func regsPlugin(t *testing.T, tag string) string {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod": "module example.com/p\n\ngo 1.26\n",
		"p.go": `package p

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"runtime/pprof"
	"strings"
)

type T struct{ Tag string }

type U struct{ N int }

var prof = pprof.NewProfile("example.com/p.conns")

func init() {
	gob.Register(T{})
	gob.RegisterName("p.U", U{})
}

// Round encodes a T and a U in interfaces and decodes them.
func Round() string {
	var buf bytes.Buffer
	in := []any{T{Tag: "` + tag + `"}, U{N: 7}}
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		return "encode: " + err.Error()
	}
	var out []any
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		return "decode: " + err.Error()
	}
	t, ok1 := out[0].(T)
	u, ok2 := out[1].(U)
	return fmt.Sprint(t.Tag, " ", ok1, " ", u.N, " ", ok2)
}

// Prof adds a value to the profile and looks it up.
func Prof() string {
	k := "conn"
	prof.Add(k, 0)
	defer prof.Remove(k)
	ps := pprof.Profiles()
	names := ""
	for _, p := range ps {
		names += p.Name() + ","
	}
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	heap := pprof.Lookup("heap") != nil
	return fmt.Sprint(pprof.Lookup("example.com/p.conns") == prof, " ", prof.Count(), " ", heap, " ", pprof.Lookup("goroutine").Count(), " ", strings.HasPrefix(buf.String(), "goroutine profile: total 0\n"), " ", names)
}
`,
	})
	return dir
}

func callString(t *testing.T, p *Plugin, name string) string {
	t.Helper()
	res, err := p.Call(context.Background(), name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	v, err := res[0].Interface()
	if err != nil {
		t.Fatal(err)
	}
	return v.(string)
}

// TestPluginRegistriesReload: a plugin that calls gob.Register,
// gob.RegisterName and pprof.NewProfile in its init loads again after
// Close (a reload of the plugin), and two plugins that register the same
// names load side by side, each with its own types. The registries of gob
// and pprof are tables of the process: the second load failed with "gob:
// registering duplicate types for ...: p.T != p.T" (the AOT copy of gob,
// AOT and default mode) and "pprof: NewProfile name already in use", and
// plugin A's names blocked plugin B. pprof.Lookup("goroutine") gave the
// plugin the stacks of every goroutine of the host.
func TestPluginRegistriesReload(t *testing.T) {
	if aotPkg("encoding/gob") == nil {
		t.Skip("no AOT copy of encoding/gob in this build")
	}
	a, b := regsPlugin(t, "a"), regsPlugin(t, "b")
	// the built-in profiles and the plugin's (goroutine is a stand-in
	// with no entry; goroutineleak exists with a GOEXPERIMENT only)
	profNames := "allocs,block,example.com/p.conns,goroutine,"
	if pprof.Lookup("goroutineleak") != nil {
		profNames += "goroutineleak,"
	}
	profNames += "heap,mutex,threadcreate,"
	rel := gorelease.Release(runtime.Version())
	for _, mode := range []string{"default", "aot", "native gob"} {
		t.Run(mode, func(t *testing.T) {
			aot := mode == "aot"
			var hooks *loadHooks
			if mode == "native gob" {
				// stale copies: encoding/gob is native (gob.go)
				hooks = fakeAOT(t, rel+".98", rel+".99")
			}
			want := func(tag string) string { return tag + " true 7 true" }
			wantProf := "true 1 true 0 true " + profNames
			for i := 0; i < 2; i++ {
				p, err := Load(Options{Dir: a, AOT: aot, hooks: hooks}, "example.com/p")
				if err != nil {
					t.Fatalf("load %d: %v", i+1, err)
				}
				if got := callString(t, p, "Round"); got != want("a") {
					t.Errorf("load %d: Round() = %q, want %q", i+1, got, want("a"))
				}
				if got := callString(t, p, "Prof"); got != wantProf {
					t.Errorf("load %d: Prof() = %q, want %q", i+1, got, wantProf)
				}
				if err := p.Close(); err != nil {
					t.Fatal(err)
				}
			}
			pa, err := Load(Options{Dir: a, AOT: aot, hooks: hooks}, "example.com/p")
			if err != nil {
				t.Fatal(err)
			}
			defer pa.Close()
			pb, err := Load(Options{Dir: b, AOT: aot, hooks: hooks}, "example.com/p")
			if err != nil {
				t.Fatalf("second plugin with the same names: %v", err)
			}
			defer pb.Close()
			for _, c := range []struct {
				p   *Plugin
				tag string
			}{{pa, "a"}, {pb, "b"}, {pa, "a"}} {
				if got := callString(t, c.p, "Round"); got != want(c.tag) {
					t.Errorf("plugin %s: Round() = %q, want %q", c.tag, got, want(c.tag))
				}
				if got := callString(t, c.p, "Prof"); got != wantProf {
					t.Errorf("plugin %s: Prof() = %q, want %q", c.tag, got, wantProf)
				}
			}
			if pprof.Lookup("example.com/p.conns") != nil {
				t.Errorf("the profile of a plugin is in the table of the process")
			}
		})
	}
}

// gobNativePlugin writes a plugin whose init registers the native type
// map[string]any with gob when register is true.
func gobNativePlugin(t *testing.T, register bool) string {
	dir := t.TempDir()
	reg := ""
	if register {
		reg = "func init() { gob.Register(map[string]any{}) }\n"
	}
	writeTree(t, dir, map[string]string{
		"go.mod": "module example.com/p\n\ngo 1.26\n",
		"p.go": `package p

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

` + reg + `
// Round encodes a map[string]any in an interface and decodes it.
func Round() (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint("panic: ", r)
		}
	}()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode([]any{map[string]any{"k": 1}}); err != nil {
		return "encode: " + err.Error()
	}
	var out []any
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		return "decode: " + err.Error()
	}
	return fmt.Sprint(out[0])
}

// RegisterOther registers map[string]any under another name.
func RegisterOther() (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint("panic: ", r)
		}
	}()
	gob.RegisterName("other.M", map[string]any{})
	return "ok"
}
`,
	})
	return dir
}

// TestPluginGobNativeTypes: the gob registrations of a native type
// (map[string]any) by one plugin are not seen by another plugin. The name
// of a native type went to the table of the machine, but the type to the
// shared table: in plugin B, Encode found the name of plugin A, then
// Decode failed ("name not registered for interface"), where Go fails at
// Encode; and a RegisterName of the type under another name in B panicked
// ("registering duplicate names").
func TestPluginGobNativeTypes(t *testing.T) {
	if aotPkg("encoding/gob") == nil {
		t.Skip("no AOT copy of encoding/gob in this build")
	}
	a, b := gobNativePlugin(t, true), gobNativePlugin(t, false)
	for _, mode := range []string{"default", "aot"} {
		t.Run(mode, func(t *testing.T) {
			aot := mode == "aot"
			pa, err := Load(Options{Dir: a, AOT: aot}, "example.com/p")
			if err != nil {
				t.Fatal(err)
			}
			defer pa.Close()
			pb, err := Load(Options{Dir: b, AOT: aot}, "example.com/p")
			if err != nil {
				t.Fatal(err)
			}
			defer pb.Close()
			const roundOK = "map[k:1]"
			if got := callString(t, pa, "Round"); got != roundOK {
				t.Errorf("plugin A: Round() = %q, want %q", got, roundOK)
			}
			wantB := "encode: gob: type not registered for interface: map[string]interface {}"
			if got := callString(t, pb, "Round"); got != wantB {
				t.Errorf("plugin B: Round() = %q, want %q", got, wantB)
			}
			if got := callString(t, pb, "RegisterOther"); got != "ok" {
				t.Errorf("plugin B: RegisterOther() = %q, want ok", got)
			}
			if got := callString(t, pb, "Round"); got != roundOK {
				t.Errorf("plugin B after RegisterName: Round() = %q, want %q", got, roundOK)
			}
			if got := callString(t, pa, "Round"); got != roundOK {
				t.Errorf("plugin A after B's RegisterName: Round() = %q, want %q", got, roundOK)
			}
		})
	}
}
