// Command loadcheck loads Go packages with the front end of yaegi v2 (v1) and
// builds their SSA code, without running anything. gotestdir uses it for the
// compile-like tests of GOROOT/test (compile, compiledir, build, builddir):
// yaegi v2 has no "load only" mode, and these tests must not run (many have a
// main function that loops or exits).
//
//	loadcheck <file.go|dir|pattern>...
//
// It does what vm.RunMain does before the program starts (vm/run.go, load
// and loadMain): go/packages with LoadSyntax, again with LoadAllSyntax when a
// package outside the standard library is involved (several packages of a
// test directory), then go/ssa with InstantiateGenerics. It does not do the
// checks of yaegi v2 that need its native symbol tables, and it does not compile
// bytecode: yaegi v2 compiles a function at its first call.
//
// Exit codes are those of yaegi v2: 0 the packages load and build, 4 load, parse
// or type error (the errors are printed one per line, as yaegi v2 prints them),
// 2 crash of the SSA builder.
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: loadcheck <file.go|dir|pattern>...")
		os.Exit(2)
	}
	os.Exit(check(os.Args[1:]))
}

func check(patterns []string) (code int) {
	pkgs, err := load(patterns, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 4
	}
	allSyntax := len(pkgs) > 1 || len(nonStdDeps(pkgs)) > 0
	if allSyntax {
		if pkgs, err = load(patterns, true); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 4
		}
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "panic:", r)
			code = 2
		}
	}()
	if allSyntax {
		prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
		prog.Build()
	} else {
		_, spkgs := ssautil.Packages(pkgs, ssa.InstantiateGenerics)
		for _, p := range spkgs {
			if p != nil {
				p.Build()
			}
		}
	}
	return 0
}

// load is vm.load (vm/run.go) for one or more packages.
func load(patterns []string, allSyntax bool) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.LoadSyntax | packages.NeedModule,
		Env:  append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH),
	}
	if allSyntax {
		cfg.Mode = packages.LoadAllSyntax | packages.NeedModule
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load: %v", err)
	}
	var msgs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			msgs = append(msgs, e.Error())
		}
	})
	if len(msgs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(msgs, "\n"))
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("load: no package matches %q", patterns)
	}
	return pkgs, nil
}

// nonStdDeps is vm.nonStdDeps: the dependencies of pkgs outside the
// standard library.
func nonStdDeps(pkgs []*packages.Package) map[string]bool {
	deps := map[string]bool{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, root := range pkgs {
			if p == root {
				return
			}
		}
		path := p.PkgPath
		if path == "" {
			path = p.ID
		}
		first, _, _ := strings.Cut(path, "/")
		if p.Module != nil || strings.Contains(first, ".") {
			deps[path] = true
		}
	})
	return deps
}
