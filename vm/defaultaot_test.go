package vm

import (
	"go/types"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestDefaultAOTPlugin loads in default mode (vm.Load, Options.AOT false) a
// plugin that gives its own types to text/template, html/template and
// log/slog. The standard library then runs as in AOT mode (stdAOTModes):
// the template calls a method of the data, and slog calls LogValue and
// keeps the json tags. With the native packages the template failed
// ("can't evaluate field Greeting") and slog printed the struct as it is.
func TestDefaultAOTPlugin(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/plug\n\ngo 1.22\n",
		"plug.go": `package plug

import (
	"context"
	htmpl "html/template"
	"log/slog"
	"net/http"
	"text/template"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type User struct {
	ID   int    ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

func (u User) Greeting() string { return "hi " + u.Name }

func (u User) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("id", u.ID), slog.String("name", u.Name))
}

type Plain struct {
	ID int ` + "`json:\"id\"`" + `
}

var (
	tt = template.Must(template.New("t").Parse("{{.Greeting}};"))
	ht = htmpl.Must(htmpl.New("h").Funcs(htmpl.FuncMap{"up": func(u User) string { return "<" + u.Name + ">" }}).Parse("{{up .}};"))
)

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := User{ID: 1, Name: "ann"}
		if err := tt.Execute(w, u); err != nil {
			w.Write([]byte(err.Error()))
		}
		if err := ht.Execute(w, u); err != nil {
			w.Write([]byte(err.Error()))
		}
		h := slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		}})
		slog.New(h).Info("x", "user", u, "plain", Plain{2})
	}), nil
}
`,
	})
	want := `hi ann;&lt;ann&gt;;{"level":"INFO","msg":"x","user":{"id":1,"name":"ann"},"plain":{"id":2}}` + "\n"
	if got := serveOnce(t, Options{Dir: dir}, "example.com/plug", "/"); got != want {
		t.Errorf("default mode:\n got %q\nwant %q", got, want)
	}
	// With the go list loader too.
	if got := serveOnce(t, Options{Dir: dir, GoList: true}, "example.com/plug", "/"); got != want {
		t.Errorf("default mode, go list:\n got %q\nwant %q", got, want)
	}
}

// TestStdAOTModes checks which modes stdAOTModes gives: the standard
// packages with a copy go to ModeAOT when the source imports a trigger,
// not in AOT mode, not without a trigger, and an explicit mode stays.
func TestStdAOTModes(t *testing.T) {
	if aotPkg("encoding/json") == nil || aotPkg("log/slog") == nil {
		t.Skip("no AOT copies")
	}
	src := func(imports ...string) []sourcePkg {
		p := types.NewPackage("example.com/p", "p")
		var imps []*types.Package
		for _, path := range imports {
			imps = append(imps, types.NewPackage(path, path))
		}
		p.SetImports(imps)
		return []sourcePkg{{types: p}}
	}
	host := (*loadHooks)(nil).hostGoVersion()
	noSrc := func(string) bool { return false }
	if got := stdAOTModes(nil, false, src("strings", "encoding/json"), host, false, noSrc, false); got != nil {
		t.Errorf("no trigger: %v, want nil", got)
	}
	got := stdAOTModes(nil, false, src("log/slog"), host, false, noSrc, false)
	for _, path := range []string{"log/slog", "encoding/json", "fmt", "text/template"} {
		if got[path] != ModeAOT {
			t.Errorf("trigger: %s has mode %v, want ModeAOT", path, got[path])
		}
	}
	if got[testedNonStd()] != ModeDefault {
		t.Errorf("trigger: a package outside the standard library changed mode")
	}
	modes := map[string]Mode{"encoding/json": ModeNative}
	got = stdAOTModes(modes, false, src("text/template"), host, false, noSrc, false)
	if got["encoding/json"] != ModeNative || got["fmt"] != ModeAOT || len(modes) != 1 {
		t.Errorf("explicit json: %v (modes %v)", got, modes)
	}
	if got := stdAOTModes(map[string]Mode{"log/slog": ModeNative}, false, src("log/slog"), host, false, noSrc, false); got["fmt"] != ModeDefault {
		t.Errorf("explicit trigger: %v, want no change", got)
	}
	if got := stdAOTModes(nil, true, src("log/slog"), host, false, noSrc, false); got != nil {
		t.Errorf("AOT mode: %v, want nil", got)
	}
	// A trigger interpreted from its source (Options.Src) is not a trigger,
	// and a package of Src keeps its source when another trigger applies.
	isSlog := func(p string) bool { return p == "log/slog" }
	if got := stdAOTModes(nil, false, src("log/slog"), host, false, isSlog, false); got != nil {
		t.Errorf("trigger from source: %v, want nil", got)
	}
	got = stdAOTModes(nil, false, src("log/slog", "text/template"), host, false, isSlog, false)
	if got["log/slog"] != ModeDefault || got["fmt"] != ModeAOT || got["text/template"] != ModeAOT {
		t.Errorf("slog from source, template trigger: %v", got)
	}
}

// testedNonStd returns a package outside the standard library that has an
// AOT copy, or "".
func testedNonStd() string {
	for _, p := range AOTPackages() {
		if !isStdPath(p) {
			return p
		}
	}
	return ""
}
