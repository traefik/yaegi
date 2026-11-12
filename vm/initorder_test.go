package vm

import (
	"context"
	"strings"
	"testing"
)

// TestPluginInitOrder: the packages of a plugin are initialized in the
// order of gc (sorted by import path, each one when its imports are
// initialized), not in the order of the import declarations, with both
// loaders and in AOT mode (GOROOT test fixedbugs/issue31636.go,
// testdata/initorder.go, testdata/initregistry.go and testdata/initleaf.go
// for RunMain). The order of gc depends on the paths, so the programs are
// tried in several modules; the wanted orders are the ones of go run.
func TestPluginInitOrder(t *testing.T) {
	// rec makes a package that records its init in package rec.
	rec := func(mod, name string, imports ...string) string {
		src := "package " + name + "\n\nimport (\n\t\"" + mod + "/rec\"\n"
		for _, imp := range imports {
			src += "\t_ \"" + imp + "\"\n"
		}
		return src + ")\n\nfunc init() { rec.Add(\"" + name + "\") }\n"
	}
	recPkg := "package rec\n\nvar Order string\n\nfunc Add(s string) { Order += s + \" \" }\n\nfunc init() { Add(\"rec\") }\n"
	plug := func(mod string, imports ...string) string {
		src := "package plug\n\nimport (\n"
		for _, imp := range imports {
			src += "\t_ \"" + mod + "/" + imp + "\"\n"
		}
		return src + "\t\"" + mod + "/rec\"\n)\n\nfunc init() { rec.Add(\"plug\") }\n\nfunc Order() string { return rec.Order }\n"
	}
	type program struct {
		name, mod string
		files     func(mod string) map[string]string
		want      string
	}
	// The packages c, b, a wait for nothing but rec: issue31636.go.
	declOrder := func(mod string) map[string]string {
		return map[string]string{
			"a/a.go":       rec(mod, "a"),
			"b/b.go":       rec(mod, "b"),
			"c/c.go":       rec(mod, "c"),
			"plug/plug.go": plug(mod, "c", "b", "a"),
		}
	}
	// A registry filled by a and b: a imports strings, b imports
	// unicode/utf8. strings waits for unicode (an init task) and the
	// packages that strings imports, which its export data does not all
	// list; unicode/utf8 has no init task, so b does not wait for it.
	registry := func(mod string) map[string]string {
		return map[string]string{
			"a/a.go":       rec(mod, "a", "strings"),
			"b/b.go":       rec(mod, "b", "unicode/utf8"),
			"plug/plug.go": plug(mod, "b", "a"),
		}
	}
	// The same with a that imports unicode, which has init work.
	registryUnicode := func(mod string) map[string]string {
		return map[string]string{
			"a/a.go":       rec(mod, "a", "unicode"),
			"b/b.go":       rec(mod, "b", "unicode/utf8"),
			"plug/plug.go": plug(mod, "b", "a"),
		}
	}
	// a imports unicode/utf8 and b imports z: they have no init task, and
	// a and b do not wait for them.
	leaf := func(mod string) map[string]string {
		return map[string]string{
			"a/a.go":       rec(mod, "a", "unicode/utf8"),
			"b/b.go":       rec(mod, "b", mod+"/z"),
			"c/c.go":       rec(mod, "c"),
			"z/z.go":       "package z\n\nconst Z = 1\n\nvar L = []string{\"x\"}\n\nvar F = func() int { return Z }\n",
			"plug/plug.go": plug(mod, "c", "b", "a"),
		}
	}
	programs := []program{
		{"decl", "example.com/t", declOrder, "rec a b c plug"},
		{"registry", "t", registry, "rec b a plug"},
		{"registry", "x", registry, "rec a b plug"},
		{"registry", "unicode/x", registry, "rec a b plug"},
		{"registryunicode", "t", registryUnicode, "rec b a plug"},
		{"registryunicode", "example.com/m", registryUnicode, "rec b a plug"},
		{"registryunicode", "x", registryUnicode, "rec a b plug"},
		{"leaf", "t", leaf, "rec a b c plug"},
		{"leaf", "example.com/m", leaf, "rec a b c plug"},
		{"leaf", "x", leaf, "rec a b c plug"},
	}
	for _, pr := range programs {
		files := pr.files(pr.mod)
		files["go.mod"] = "module " + pr.mod + "\n\ngo 1.22\n"
		files["rec/rec.go"] = recPkg
		dir := writeModule(t, files)
		for _, tc := range []struct {
			name string
			opts Options
		}{
			{"default", Options{Dir: dir}},
			{"golist", Options{Dir: dir, GoList: true}},
			{"aot", Options{Dir: dir, AOT: true}},
		} {
			t.Run(pr.name+"/"+pr.mod+"/"+tc.name, func(t *testing.T) {
				p, err := Load(tc.opts, pr.mod+"/plug")
				if err != nil {
					t.Fatal(err)
				}
				res, err := p.Call(context.Background(), "Order")
				if err != nil {
					t.Fatal(err)
				}
				got, err := res[0].Interface()
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.TrimSpace(got.(string)); got != pr.want {
					t.Errorf("init order %q, want %q (go run)", got, pr.want)
				}
			})
		}
	}
}
