package vm

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// TestAOTRoutesStaleCopies checks that default mode routes no call to an
// AOT copy that cannot run in this host. The routes of aotroute.go (fmt,
// encoding/json, encoding/xml) and of the stream methods of json send a
// call with a plugin value to the copy: with copies made with an older Go
// patch release than the host (a host built with a newer Go patch, with
// or without StaleAOTFallback), json.Unmarshal, fmt.Sprintf and
// xml.Marshal ran the copies, which miss the fixes of the host. Now such a
// call stays native, and a value whose methods native json would not call
// fails the load with the reason. With fresh copies, the calls go to the
// copies.
func TestAOTRoutesStaleCopies(t *testing.T) {
	if aotPkg("encoding/json") == nil || aotPkg("fmt") == nil || aotPkg("encoding/xml") == nil {
		t.Skip("no AOT copies of fmt, encoding/json and encoding/xml in this build")
	}
	plugin := func(src string) string {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{
			"go.mod":  "module example.com/plug\n\ngo 1.26\n",
			"plug.go": "package plug\n\n" + src,
		})
		return dir
	}
	// json: a method that native json does not call (in a map).
	jdir := plugin(`import (
	"encoding/json"
	"fmt"
	"strings"
)

type T struct{ A int }

func (t *T) UnmarshalJSON(b []byte) error { t.A = len(b); return nil }

type W struct{ In map[string]T }

func JSON() string {
	var w W
	err := json.Unmarshal([]byte(` + "`" + `{"In":{"k":[1,2]}}` + "`" + `), &w)
	return fmt.Sprint(w.In["k"].A, " ", err)
}

func JSONStream() string {
	var w W
	err := json.NewDecoder(strings.NewReader(` + "`" + `{"In":{"k":[1,2]}}` + "`" + `)).Decode(&w)
	return fmt.Sprint(w.In["k"].A, " ", err)
}
`)
	// fmt: the copy names the plugin's type, native fmt does not.
	fdir := plugin(`import "fmt"

type S struct{ B int }

func Fmt() string { return fmt.Sprintf("%T", S{}) }
`)
	// encoding/xml is an AOT trigger (stdAOTModes): with fresh copies, a
	// plugin that imports it runs the standard library as in AOT mode.
	// Native xml refuses the unnamed struct of a plugin type.
	xdir := plugin(`import (
	"encoding/xml"
	"fmt"
)

type X struct{ C int }

func XML() string {
	b, err := xml.Marshal(X{C: 1})
	return string(b) + " " + fmt.Sprint(err)
}
`)
	rel := gorelease.Release(runtime.Version())
	call := func(t *testing.T, p *Plugin, name string) string {
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
	t.Run("stale", func(t *testing.T) {
		for _, opts := range []Options{
			{},
			{AOT: true, StaleAOTFallback: true},
		} {
			opts.hooks = fakeAOT(t, rel+".98", rel+".99")
			opts.Dir = jdir
			_, err := Load(opts, "example.com/plug")
			if err == nil || strings.Count(err.Error(), "the AOT copy of encoding/json cannot run in this host") != 2 {
				t.Errorf("AOT %v: json plugin: %v, want two load errors that say the AOT copy is stale", opts.AOT, err)
			}
			// Each problem gives the two Go versions, not the whole
			// reason of the stale copies, which Plugin.Notes and
			// CheckAOT give once (about 600 characters per problem
			// before).
			if err != nil && (strings.Contains(err.Error(), "miss the fixes") || !strings.Contains(err.Error(), "and the host is built with "+rel+".99; Plugin.Notes")) {
				t.Errorf("AOT %v: json plugin: %v, want the short reason of the stale copies", opts.AOT, err)
			}
			for _, c := range []struct{ dir, fn, path, copyOut string }{
				{fdir, "Fmt", "fmt", "plug.S"},
				{xdir, "XML", "encoding/xml", "<X><C>1</C></X> <nil>"},
			} {
				opts.Dir = c.dir
				p, err := Load(opts, "example.com/plug")
				if err != nil {
					t.Fatal(err)
				}
				if len(p.M.routed) != 0 || p.M.modeOf(c.path) != ModeNative {
					t.Errorf("AOT %v: routed %v, mode of %s %v, want no route and native", opts.AOT, p.M.routed, c.path, p.M.modeOf(c.path))
				}
				if got := call(t, p, c.fn); got == c.copyOut {
					t.Errorf("AOT %v: %s() = %q, ran the stale AOT copy", opts.AOT, c.fn, got)
				}
			}
		}
	})
	t.Run("fresh", func(t *testing.T) {
		vs := AOTGoVersions()
		h := fakeAOT(t, vs[len(vs)-1], vs[len(vs)-1])
		for _, c := range []struct {
			dir, path string
			want      map[string]string
		}{
			{jdir, "encoding/json", map[string]string{"JSON": "5 <nil>", "JSONStream": "5 <nil>"}},
			{fdir, "fmt", map[string]string{"Fmt": "plug.S"}},
			{xdir, "", map[string]string{"XML": "<X><C>1</C></X> <nil>"}},
		} {
			p, err := Load(Options{Dir: c.dir, hooks: h}, "example.com/plug")
			if err != nil {
				t.Fatal(err)
			}
			if c.path != "" && !p.M.routed[c.path] {
				t.Errorf("%s is not routed to its AOT copy", c.path)
			}
			for name, want := range c.want {
				if got := call(t, p, name); got != want {
					t.Errorf("%s() = %q, want %q", name, got, want)
				}
			}
		}
	})
}
