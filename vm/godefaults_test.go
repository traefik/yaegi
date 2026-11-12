package vm

// The zero Options accept a plugin only when go build would build it: the
// options that accept more (for plugins written for yaegi) are off by
// default. These tests check each of them with and without the option, and
// compare the default with go build on real programs.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// handlerPlugin returns the source of a plugin package whose handler writes
// body, with extra declarations after it.
func handlerPlugin(pkg, imports, body, extra string) string {
	return "package " + pkg + `

import (
	"context"
	"fmt"
	"net/http"
` + imports + `)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, ` + body + `)
	}), nil
}
` + extra
}

// TestAllowNewerGo checks Options.AllowNewerGo: a go.mod (or a vendored
// module) that asks for a Go release newer than the one yaegi v2 was built with
// refuses the plugin, as the go command refuses to build it with the release
// of the host; with the option, the plugin is type checked as the release of
// the host and runs. A newer patch level of the same release is not a newer
// release. Before, the directive was always lowered without a word.
func TestAllowNewerGo(t *testing.T) {
	needSource(t)
	for name, files := range map[string]map[string]string{
		"go.mod": {
			"go.mod":  "module example.com/plug\n\ngo 1.99\n",
			"plug.go": handlerPlugin("plug", "", `"new"`, ""),
		},
		// A go.mod of the newer release may have a directive the host does
		// not know: it is read as the go command reads it (ParseLax), and
		// the option accepts it.
		"unknown directive": {
			"go.mod":  "module example.com/plug\n\ngo 1.99\n\nfuture something\n",
			"plug.go": handlerPlugin("plug", "", `"new"`, ""),
		},
		"vendor": {
			"go.mod":                        "module example.com/plug\n\ngo 1.22\n\nrequire example.com/dep v1.0.0\n",
			"vendor/modules.txt":            "# example.com/dep v1.0.0\n## explicit; go 1.99\nexample.com/dep\n",
			"vendor/example.com/dep/dep.go": "package dep\n\nconst V = \"new\"\n",
			"plug.go":                       handlerPlugin("plug", "\t\"example.com/dep\"\n", "dep.V", ""),
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeModule(t, files)
			_, err := Load(Options{Dir: dir}, "example.com/plug")
			if err == nil || !strings.Contains(err.Error(), "asks for go 1.99") {
				t.Fatalf("without AllowNewerGo: %v, want the newer go directive refused", err)
			}
			if got := serveOnce(t, Options{Dir: dir, AllowNewerGo: true}, "example.com/plug", "/"); got != "new" {
				t.Errorf("with AllowNewerGo: body %q, want %q", got, "new")
			}
		})
	}
	// The same release with a patch level: not newer.
	dir := writeModule(t, map[string]string{
		"go.mod":  "module example.com/plug\n\ngo 1.22.99\n",
		"plug.go": handlerPlugin("plug", "", `"patch"`, ""),
	})
	if got := serveOnce(t, Options{Dir: dir}, "example.com/plug", "/"); got != "patch" {
		t.Errorf("patch level: body %q, want %q", got, "patch")
	}
	// An unknown directive in a go.mod of the release of the host (or an
	// older one) is an error of the file, with or without the option.
	dir = writeModule(t, map[string]string{
		"go.mod":  "module example.com/plug\n\ngo 1.22\n\nfuture something\n",
		"plug.go": handlerPlugin("plug", "", `"x"`, ""),
	})
	for _, opts := range []Options{{Dir: dir}, {Dir: dir, AllowNewerGo: true}} {
		if _, err := Load(opts, "example.com/plug"); err == nil || !strings.Contains(err.Error(), "unknown directive: future") {
			t.Errorf("unknown directive, AllowNewerGo=%v: %v, want the directive refused", opts.AllowNewerGo, err)
		}
	}
}

// TestNoGoDirective checks a go.mod without a go directive: the go command
// builds such a module as go 1.16 (gover.DefaultGoModVersion), so a plugin
// that uses a newer feature of the language is refused, as go build refuses
// it, and its vendor directory is checked as the one of a go 1.16 module
// (explicit marks required). Before, the loader used the release of the
// host, and skipped the explicit marks as for a go.mod older than 1.14.
func TestNoGoDirective(t *testing.T) {
	needSource(t)
	dir := writeModule(t, map[string]string{
		"go.mod":  "module example.com/plug\n",
		"plug.go": handlerPlugin("plug", "", `id("x")`, "func id[T any](t T) T { return t }\n"),
	})
	if _, err := Load(Options{Dir: dir}, "example.com/plug"); err == nil || !strings.Contains(err.Error(), "requires go1.18 or later") {
		t.Errorf("generics without a go directive: %v, want them refused as go1.16", err)
	}
	if goCmd, err := exec.LookPath("go"); err == nil {
		cmd := exec.Command(goCmd, "build", "./...")
		cmd.Dir = dir
		// -mod=readonly: with -mod=mod, go build adds a go directive.
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=readonly", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=local")
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "go1.16") {
			t.Errorf("go build: %v\n%s, want the generics refused as go1.16", err, out)
		}
	}
	dir = writeModule(t, map[string]string{
		"go.mod":                        "module example.com/plug\n\nrequire example.com/dep v1.0.0\n",
		"vendor/modules.txt":            "# example.com/dep v1.0.0\nexample.com/dep\n",
		"vendor/example.com/dep/dep.go": "package dep\n\nconst V = \"dep\"\n",
		"plug.go":                       handlerPlugin("plug", "\t\"example.com/dep\"\n", "dep.V", ""),
	})
	if _, err := Load(Options{Dir: dir}, "example.com/plug"); err == nil || !strings.Contains(err.Error(), "not marked as explicit") {
		t.Errorf("vendor without explicit marks and no go directive: %v, want it refused as go 1.16", err)
	}
	if got := serveOnce(t, Options{Dir: dir, IgnoreInconsistentVendoring: true}, "example.com/plug", "/"); got != "dep" {
		t.Errorf("with IgnoreInconsistentVendoring: body %q, want %q", got, "dep")
	}
}

// TestImportFromArchive checks Options.ImportFromArchive: a plugin that is a
// nested module (its own go.mod below Options.Dir) and imports a package of
// the module around it that it does not vendor is refused, as go build
// refuses it; with the option, the package is read from its directory in the
// archive, as yaegi reads it. Before, it was always read from there.
func TestImportFromArchive(t *testing.T) {
	needSource(t)
	dir := writeModule(t, map[string]string{
		"go.mod":         "module example.com/root\n\ngo 1.22\n",
		"lib/lib.go":     "package lib\n\nconst Name = \"root lib\"\n",
		"plugin/go.mod":  "module example.com/root/plugin\n\ngo 1.22\n",
		"plugin/plug.go": handlerPlugin("plugin", "\t\"example.com/root/lib\"\n", "lib.Name", ""),
	})
	_, err := Load(Options{Dir: dir}, "example.com/root/plugin")
	if err == nil || !strings.Contains(err.Error(), "cannot find package example.com/root/lib") || !strings.Contains(err.Error(), "ImportFromArchive") {
		t.Fatalf("without ImportFromArchive: %v, want the import refused", err)
	}
	if got := serveOnce(t, Options{Dir: dir, ImportFromArchive: true}, "example.com/root/plugin", "/"); got != "root lib" {
		t.Errorf("with ImportFromArchive: body %q, want %q", got, "root lib")
	}
}

// TestIgnoreInconsistentVendoring checks Options.IgnoreInconsistentVendoring:
// a vendor/modules.txt that does not match go.mod refuses the plugin with
// the messages of go build ("inconsistent vendoring"); with the option, the
// vendor directory is read as it is, as yaegi reads it. A module that
// matches loads without the option. Before, go.mod was never compared with
// modules.txt.
func TestIgnoreInconsistentVendoring(t *testing.T) {
	needSource(t)
	const goMod = "module example.com/plug\n\ngo 1.22\n\nrequire example.com/dep v1.0.0\n"
	const modulesTxt = "# example.com/dep v1.0.0\n## explicit; go 1.20\nexample.com/dep\n"
	files := func(goMod, modulesTxt string) map[string]string {
		return map[string]string{
			"go.mod":                        goMod,
			"vendor/modules.txt":            modulesTxt,
			"vendor/example.com/dep/dep.go": "package dep\n\nconst V = \"dep\"\n",
			"plug.go":                       handlerPlugin("plug", "\t\"example.com/dep\"\n", "dep.V", ""),
		}
	}
	consistent := writeModule(t, files(goMod, modulesTxt))
	if got := serveOnce(t, Options{Dir: consistent}, "example.com/plug", "/"); got != "dep" {
		t.Errorf("consistent vendoring: body %q, want %q", got, "dep")
	}
	for _, tc := range []struct {
		name, goMod, modulesTxt, want string
	}{
		{"not required", "module example.com/plug\n\ngo 1.22\n", modulesTxt,
			"example.com/dep@v1.0.0: is marked as explicit in vendor/modules.txt, but not explicitly required in go.mod"},
		{"not explicit", goMod, "# example.com/dep v1.0.0\nexample.com/dep\n",
			"example.com/dep@v1.0.0: is explicitly required in go.mod, but not marked as explicit in vendor/modules.txt"},
		{"no modules.txt", goMod, "",
			"example.com/dep@v1.0.0: is explicitly required in go.mod, but not marked as explicit in vendor/modules.txt"},
		{"other version", goMod, "# example.com/dep v1.1.0\n## explicit; go 1.20\nexample.com/dep\n",
			"example.com/dep@v1.1.0: is marked as explicit in vendor/modules.txt, but not explicitly required in go.mod"},
		{"replaced in go.mod only", goMod + "\nreplace example.com/dep => example.com/fork v1.0.0\n", modulesTxt,
			"example.com/dep: is replaced in go.mod, but not marked as replaced in vendor/modules.txt"},
		{"replaced in modules.txt only", goMod, "# example.com/dep v1.0.0 => example.com/fork v1.0.0\n## explicit; go 1.20\nexample.com/dep\n",
			"example.com/dep@v1.0.0: is marked as replaced in vendor/modules.txt, but not replaced in go.mod"},
		// Before go 1.14, modules.txt had no explicit marks: only a version
		// that differs is an error.
		{"pre-1.14 version", "module example.com/plug\n\ngo 1.13\n\nrequire example.com/dep v1.0.0\n", "# example.com/dep v1.1.0\nexample.com/dep\n",
			"example.com/dep@v1.0.0: is explicitly required in go.mod, but vendor/modules.txt indicates example.com/dep@v1.1.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := files(tc.goMod, tc.modulesTxt)
			if tc.modulesTxt == "" {
				delete(f, "vendor/modules.txt")
			}
			dir := writeModule(t, f)
			_, err := Load(Options{Dir: dir}, "example.com/plug")
			if err == nil || !strings.Contains(err.Error(), "inconsistent vendoring") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("without IgnoreInconsistentVendoring: %v, want %q", err, tc.want)
			}
			if got := serveOnce(t, Options{Dir: dir, IgnoreInconsistentVendoring: true}, "example.com/plug", "/"); got != "dep" {
				t.Errorf("with IgnoreInconsistentVendoring: body %q, want %q", got, "dep")
			}
		})
	}
	// What the go command accepts (checkVendorConsistency of cmd/go): a
	// matching replacement (go mod vendor writes a replacement of every
	// version twice: for the version in the build list, and alone at the
	// end), a module before go 1.14 without explicit marks, an explicit
	// module without a package (go.mod requires it, or required it, for
	// its go.mod only: not in the build list, so not checked), a module
	// line the go command does not read (no semantic version), and a
	// replace of the main module itself (never applied). go build, when it
	// is there, must accept them too.
	goCmd, _ := exec.LookPath("go")
	for name, f := range map[string]map[string]string{
		"replaced":                   files(goMod+"\nreplace example.com/dep => ./fork\n", "# example.com/dep v1.0.0 => ./fork\n## explicit; go 1.20\nexample.com/dep\n# example.com/dep => ./fork\n"),
		"pre-1.14":                   files("module example.com/plug\n\ngo 1.13\n\nrequire example.com/dep v1.0.0\n", "# example.com/dep v1.0.0\nexample.com/dep\n"),
		"explicit without package":   files(goMod, modulesTxt+"# example.com/tool v1.0.0\n## explicit; go 1.20\n"),
		"line without a version":     files(goMod, "# example.com/old\n## explicit\n"+modulesTxt),
		"replace of the main module": files(goMod+"\nreplace example.com/plug => ./elsewhere\n", modulesTxt),
	} {
		dir := writeModule(t, f)
		if got := serveOnce(t, Options{Dir: dir}, "example.com/plug", "/"); got != "dep" {
			t.Errorf("%s: body %q, want %q", name, got, "dep")
		}
		if goCmd == "" || name == "pre-1.14" { // go build reads the module cache before go 1.14
			continue
		}
		cmd := exec.Command(goCmd, "build", "./...")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=vendor", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=local")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: go build refuses the module that Load accepts: %v\n%s", name, err, out)
		}
	}
}

// TestGoBuildRefusals checks, on real programs, that the zero Options refuse
// what go build refuses, and that the option made for plugins written for
// yaegi accepts each one. When the go command is there, go build is run on
// the same module and must refuse it too. Without any option, the only
// difference with go build must be a refusal that go build does not make,
// never a plugin that go build refuses and yaegi v2 accepts.
func TestGoBuildRefusals(t *testing.T) {
	needSource(t)
	goCmd, _ := exec.LookPath("go")
	plug := func(body, extra string) string { return handlerPlugin("plug", "", body, extra) }
	for _, tc := range []struct {
		name  string
		files map[string]string
		with  Options // the option that accepts the program
		want  string  // part of the error of Load without the option
		hint  string  // the option, named by the error
	}{
		{"unused variable", map[string]string{
			"plug.go": plug(`"x"`, "func f() { x := 1 }\n"),
		}, Options{IgnoreSoftErrors: true}, "declared and not used", "Options.IgnoreSoftErrors"},
		{"unused variable and expression", map[string]string{ // both options
			"plug.go": plug(`"x"`, "func f(s string) { x := 1; len(s) }\n"),
		}, Options{IgnoreSoftErrors: true, IgnoreUnusedExprs: true}, "is not used", "Options.IgnoreSoftErrors and Options.IgnoreUnusedExprs"},

		{"unused import", map[string]string{
			"plug.go": handlerPlugin("plug", "\t\"strings\"\n", `"x"`, ""),
		}, Options{IgnoreSoftErrors: true}, "imported and not used", "Options.IgnoreSoftErrors"},
		{"unused expression", map[string]string{
			"plug.go": plug(`"x"`, "func f(s string) { len(s) }\n"),
		}, Options{IgnoreUnusedExprs: true}, "is not used", "Options.IgnoreUnusedExprs"},
		{"duplicate declaration", map[string]string{ // a copy of the file left next to it
			"plug.go": plug(`"x"`, ""),
			"copy.go": plug(`"x"`, ""),
		}, Options{IgnoreDuplicateDecls: true}, "redeclared", "Options.IgnoreDuplicateDecls"},
		{"test file in another package", map[string]string{
			"plug.go":      plug(`"x"`, ""),
			"main_test.go": "package main\n",
		}, Options{IgnoreTestFiles: true}, "found packages", "Options.IgnoreTestFiles"},
		{"file selected by a yaegi:tags line", map[string]string{
			"plug.go":   "// yaegi:tags plugtag\n\n" + plug("tagged", ""),
			"tagged.go": "//go:build plugtag\n\npackage plug\n\nconst tagged = \"x\"\n",
		}, Options{YaegiTags: true}, "undefined: tagged", "Options.YaegiTags"},
		{"newer go directive", map[string]string{
			"go.mod":  "module example.com/plug\n\ngo 1.99\n",
			"plug.go": plug(`"x"`, ""),
		}, Options{AllowNewerGo: true}, "asks for go 1.99", "Options.AllowNewerGo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"go.mod": "module example.com/plug\n\ngo 1.22\n"}
			for name, src := range tc.files {
				files[name] = src
			}
			dir := writeModule(t, files)
			_, err := Load(Options{Dir: dir}, "example.com/plug")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("zero Options: %v, want an error with %q", err, tc.want)
			}
			// A plugin refused by default says which option accepts it.
			if !strings.Contains(err.Error(), tc.hint) {
				t.Errorf("zero Options: %v, want the error to name %s", err, tc.hint)
			}
			// Not when another error refuses it with the option.
			if tc.name == "unused variable" {
				files["more.go"] = "package plug\n\nvar s string = 1\n"
				dir := writeModule(t, files)
				if _, err := Load(Options{Dir: dir}, "example.com/plug"); err == nil || !strings.Contains(err.Error(), "cannot use 1") || strings.Contains(err.Error(), "Options.") {
					t.Errorf("zero Options, with another error: %v, want no option named", err)
				}
			}
			opts := tc.with
			opts.Dir = dir
			if _, err := Load(opts, "example.com/plug"); err != nil {
				t.Errorf("with the option: %v", err)
			}
			if goCmd == "" {
				return
			}
			cmd := exec.Command(goCmd, "build", "./...")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=local")
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Errorf("go build accepts the program that Load refuses:\n%s", out)
			}
		})
	}
}
