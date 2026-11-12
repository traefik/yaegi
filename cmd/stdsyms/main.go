// Command stdsyms generates package stdsyms: the native symbol table of the
// standard library for the Go release it runs with, with yaegi v2's extractor
// (internal/extract, derived from yaegi's), which also substitutes the
// restricted functions (stdsyms/restricted.go).
//
// It also generates the tables that only programs run without restrictions
// get (stdsyms.Unrestricted): os/exec, and syscall for stdtypes.Platforms
// (the API of syscall is different on each platform: one file per
// platform, which its name selects, _GOOS_GOARCH.go). On other platforms,
// the VM refuses syscall at load time for those programs. The table of
// golang.org/x/sys/unix, for the unix platforms of stdtypes.Platforms, is
// made the same way (xsys.go, stdsyms.XSysUnix).
//
// The files are only built by the Go release they are generated with
// ("go1.26 && !go1.27"), and release_guard.go stops the build on any other
// release (see internal/gorelease).
//
//	go generate ./stdsyms     # runs: go run ../cmd/stdsyms
package main

import (
	"flag"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/traefik/yaegi/v2/internal/extract"
	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// excluded are the standard library packages without a table in
// stdsyms.Symbols, the table of every machine. os/exec and syscall are not in
// it because plugins never get them (yaegi's restricted stdlib): main writes
// their tables to stdsyms.Unrestricted, which only the programs with the
// standard library as in Go get. plugin and unsafe have no table at all
// (plugin is always refused; the functions of unsafe are builtins). The
// others are of no use to an interpreter (cgo, sanitizers, time zone data,
// test-only packages). embed has a table: the VM fills an embed.FS variable
// of a package run from source with its //go:embed files (vm/embed.go).
var excluded = map[string]bool{
	"os/exec": true, "plugin": true, "unsafe": true, "syscall": true,
	"runtime/cgo": true, "runtime/race": true, "runtime/msan": true, "runtime/asan": true, "runtime/coverage": true,
	"time/tzdata": true, "testing/cryptotest": true, "crypto/mlkem/mlkemtest": true,
}

// sideEffects are the standard library packages whose init changes the
// global state of the process: expvar and net/http/pprof register handlers
// on http.DefaultServeMux (/debug/vars, /debug/pprof/), and testing/quick
// defines the flag -quickchecks on flag.CommandLine. Their tables go to
// package stdsyms/sideeffect, so that a host that imports the VM does not
// get these side effects: a host that wants them imports that package
// (cmd/yaegi does). Restricted plugins may not import expvar and
// net/http/pprof (vm/restrict.go).
var sideEffects = map[string]bool{"expvar": true, "net/http/pprof": true, "testing/quick": true}

// constraints are build constraints of packages that only exist on some
// platforms.
var constraints = map[string]string{
	"log/syslog": "!windows && !plan9 && !js && !wasip1",
}

func main() {
	dir := flag.String("o", ".", "output directory (package stdsyms)")
	xsysOnly := flag.Bool("xsys", false, "only write the tables of golang.org/x/sys/unix (they do not depend on the Go release)")
	flag.Parse()
	if *xsysOnly {
		saved := build.Default
		nx, err := writeXSys(*dir, saved, gorelease.Platforms)
		build.Default = saved
		if err != nil {
			fatal(err)
		}
		fmt.Printf("stdsyms: golang.org/x/sys/unix for %d platforms\n", nx)
		return
	}
	release, err := gorelease.Constraint(runtime.Version())
	if err != nil {
		fatal(err)
	}
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName}, "std")
	if err != nil {
		fatal(err)
	}
	prefix := gorelease.Prefix(runtime.Version())
	// The files of the other releases of the window stay: each release of
	// gorelease.Window has its tables. The files of this release and of
	// the releases outside the window go.
	sdir := filepath.Join(*dir, "sideeffect")
	old, _ := filepath.Glob(filepath.Join(*dir, "go1_*.go"))
	old2, _ := filepath.Glob(filepath.Join(sdir, "go1_*.go"))
	for _, f := range append(old, old2...) {
		if strings.HasPrefix(filepath.Base(f), prefix+"_") || gorelease.OutsideWindow(f) {
			os.Remove(f)
		}
	}
	n := 0
	for _, p := range pkgs {
		path := p.PkgPath
		if excluded[path] || strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "vendor/") ||
			strings.HasPrefix(path, "syscall/") || strings.Contains(path, "internal") {
			continue
		}
		if sideEffects[path] {
			write(sdir, "sideeffect", prefix, release, path, "", false)
		} else {
			write(*dir, "stdsyms", prefix, release, path, "", false)
		}
		n++
	}
	write(*dir, "stdsyms", prefix, release, "os/exec", "", true)
	// The source importer of the extractor reads build.Default: change the
	// platform there to extract syscall for another platform.
	saved := build.Default
	for _, pl := range gorelease.Platforms {
		if err := setPlatform(&build.Default, saved, pl); err != nil {
			fatal(err)
		}
		write(*dir, "stdsyms", prefix, release, "syscall", "_"+build.Default.GOOS+"_"+build.Default.GOARCH, true)
	}
	nx, err := writeXSys(*dir, saved, gorelease.Platforms)
	if err != nil {
		fatal(err)
	}
	build.Default = saved
	guard := gorelease.Guard("cmd/stdsyms", "github.com/traefik/yaegi/v2/stdsyms", "go generate ./stdtypes ./stdsyms && sh scripts/aotgen.sh, with the toolchain of each release")
	if err := os.WriteFile(filepath.Join(*dir, "release_guard.go"), guard, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("stdsyms: %d packages, os/exec, syscall for %d platforms, golang.org/x/sys/unix for %d, for %s\n", n, len(gorelease.Platforms), nx, runtime.Version())
}

// write extracts the table of path into a file of dir, in package pkg
// (stdsyms, or sideeffect whose Symbols is stdsyms.Symbols). release is the
// build constraint of the Go release ("go1.26 && !go1.27"). suffix ends the
// file name (a platform: "_linux_amd64"). The table of an unrestricted file
// goes to stdsyms.Unrestricted instead of stdsyms.Symbols.
func write(dir, pkg, prefix, release, path, suffix string, unrestricted bool) {
	// One constraint line per file: the release, and the platforms.
	tag := release
	if c, ok := constraints[path]; ok {
		tag += " && " + c
	}
	table := "Symbols"
	if unrestricted {
		table = "Unrestricted"
	}
	src, err := extract.Extract(path, extract.Config{Dest: pkg, Table: table, Build: tag})
	if err != nil {
		fatal(fmt.Errorf("%s: %v", path, err))
	}
	name := filepath.Join(dir, prefix+"_"+strings.ReplaceAll(path, "/", "_")+suffix+".go")
	if err := os.WriteFile(name, src, 0o644); err != nil {
		fatal(err)
	}
}

// setPlatform sets ctxt to the build context of platform pl ("linux/386"),
// from base, the context of the machine. GOOS and GOARCH are not enough:
// the tool tags (goexperiment.regabiargs...) depend on GOARCH too. With the
// tags of an arm64 machine, internal/abi does not type check for 386 (its
// IntArgRegs is in a file for the platforms without register arguments).
// The go command gives the tags of the target. cgo is off, as in
// cmd/stdexport: the result does not depend on the machine that generates.
func setPlatform(ctxt *build.Context, base build.Context, pl string) error {
	goos, goarch, _ := strings.Cut(pl, "/")
	cmd := exec.Command("go", "list", "-f", "{{join context.ToolTags \" \"}}", "runtime")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("tool tags of %s: %v", pl, err)
	}
	*ctxt = base
	ctxt.GOOS, ctxt.GOARCH = goos, goarch
	ctxt.CgoEnabled = false
	ctxt.ToolTags = strings.Fields(string(out))
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "stdsyms:", err)
	os.Exit(1)
}
