// Command aotgen compiles Go packages "ahead of time for the VM": it copies
// a package (standard library or module cache) under aot/go1_NN/<import path>/
// (one tree per Go release of the window of yaegi v2, see release.go) with
// its reflection rewritten against the VM's Go-level reflect twin
// (github.com/traefik/yaegi/v2/vreflect), and generates the glue that lets the VM
// route interpreted calls to the copy (symbol table, native hooks, interface
// wrappers). The copy runs as native compiled code but sees the types and
// methods of the interpreted program.
//
// Usage:
//
//	aotgen [flags] importpath...
//
// Every package of the dependency closure of the targets that uses reflect
// (and is not part of the native floor: runtime, sync, os, net/http...) is
// converted too; internal packages they need are copied. The report (-report)
// lists, per package, the reflect API used and what the twin lacks.
//
// See docs/AOTGEN.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/traefik/yaegi/v2/internal/gorelease"
)

func main() {
	out := flag.String("o", "aot", "output directory of the AOT copies (relative to the module root)")
	natOut := flag.String("native-out", "natsyms", "output directory of the native symbol tables of non-std targets")
	report := flag.String("report", "", "write the report to this file (default: <o>/REPORT.txt)")
	keep := flag.String("keep", "", "comma-separated packages kept native even if they use reflect")
	noBuild := flag.Bool("nobuild", false, "do not build the generated packages")
	native := flag.Bool("native", true, "also generate native symbol tables for non-std targets (-native-out)")
	clean := flag.Bool("clean", true, "remove the output directories first")
	allowFail := flag.String("allow-fail", "", "comma-separated targets allowed not to build (aotgen exits 1 when another one does not)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: aotgen [flags] importpath...\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	root, mod, err := moduleRoot()
	if err != nil {
		fatal(err)
	}
	expr, err := gorelease.Constraint(runtime.Version())
	if err != nil {
		fatal(err)
	}
	prefix := gorelease.Prefix(runtime.Version())
	topDir, topPath := filepath.Join(root, *out), mod+"/"+filepath.ToSlash(*out)
	g := &gen{
		root:    root,
		module:  mod,
		outDir:  filepath.Join(topDir, prefix),
		outPath: topPath + "/" + prefix,
		natDir:  filepath.Join(root, *natOut),
		natPath: mod + "/" + filepath.ToSlash(*natOut),
		keep:    map[string]bool{},
	}
	for _, k := range strings.Split(*keep, ",") {
		if k != "" {
			g.keep[k] = true
		}
	}
	if *clean {
		// The tree of this release only: the trees of the other releases of
		// the window stay.
		os.RemoveAll(g.outDir)
		cleanTrees(topDir)
		if *native {
			os.RemoveAll(g.natDir)
		}
	}
	if err := g.load(flag.Args()); err != nil {
		fatal(err)
	}
	g.plan()
	if err := g.generate(); err != nil {
		fatal(err)
	}
	if *native {
		if err := g.genNative(); err != nil {
			fatal(err)
		}
	}
	if err := g.licenses(); err != nil {
		fatal(err)
	}
	if err := addRelease(g.outDir, expr); err != nil {
		fatal(err)
	}
	if !*noBuild {
		g.build()
		if err := g.quarantine(); err != nil {
			fatal(err)
		}
		if err := addRelease(g.outDir, expr); err != nil { // <o>/all again
			fatal(err)
		}
	}
	if err := writeTop(topDir, topPath, prefix, expr); err != nil {
		fatal(err)
	}
	if err := recordGoSources(topDir, g.outDir); err != nil {
		fatal(err)
	}
	rp := *report
	if rp == "" {
		rp = filepath.Join(g.outDir, "REPORT.txt")
	}
	text := g.reportText()
	if err := os.WriteFile(rp, []byte(text), 0o644); err != nil {
		fatal(err)
	}
	fmt.Print(g.summary())
	fmt.Println("report:", rp)
	if failed := g.failed(*allowFail); len(failed) > 0 {
		// The packages were left out of <o>/all: the build passes, but the
		// libraries silently lose their AOT copies.
		fatal(fmt.Errorf("packages that do not build: %s (see %s)", strings.Join(failed, ", "), rp))
	}
}

// failed returns the generated packages that do not build, except those of
// the comma-separated allow list and the packages under them.
func (g *gen) failed(allow string) []string {
	var allowed []string
	for _, a := range strings.Split(allow, ",") {
		if a != "" {
			allowed = append(allowed, a)
		}
	}
	var out []string
	for _, pi := range g.sorted() {
		if pi.kind != kConvert || pi.built || pi.buildErr == "" {
			continue
		}
		ok := false
		for _, a := range allowed {
			if pi.p.PkgPath == a || strings.HasPrefix(pi.p.PkgPath, a+"/") {
				ok = true
			}
		}
		if !ok {
			out = append(out, pi.p.PkgPath)
		}
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "aotgen:", err)
	os.Exit(1)
}

// moduleRoot finds the enclosing go.mod and its module path.
func moduleRoot() (dir, mod string, err error) {
	dir, err = os.Getwd()
	if err != nil {
		return "", "", err
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if f := strings.Fields(l); len(f) == 2 && f[0] == "module" {
					return dir, f[1], nil
				}
			}
			return "", "", fmt.Errorf("no module line in %s/go.mod", dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("no go.mod found")
		}
		dir = parent
	}
}
