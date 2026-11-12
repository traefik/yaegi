// Command boundaryscan builds the inventory of the native boundary: every
// place where an exported function of the standard library changes what it
// does because of the dynamic type, or the methods, of a value that comes
// from its caller. A plugin of yaegi v2 gives such values to the native
// standard library, and the native code must see them as compiled Go would.
//
// It reads the sources of GOROOT (the toolchain it runs with), builds the
// SSA form of the standard library for linux/amd64, and follows the values
// that come from the caller (taint):
//
//   - the sources are the parameters of the exported functions and methods
//     that can hold a value of the program (an interface, a func, a type
//     parameter, or a type that contains one), the exported fields of
//     exported structs of that kind, and what an interface method or a func
//     value of such a value returns;
//   - the taint goes through copies, fields, elements, conversions, calls
//     of the same package and stores (a field of a struct keeps the taint
//     for every value of that struct type).
//
// For each exported function or method of a package that plugins can import,
// it walks the functions of the same package (and of the internal packages
// of the standard library) that the function calls, and reports:
//
//   - assert: a type assertion or a type switch on a value of the caller
//     (x.(io.WriterTo), x.(fmt.Stringer), x.(string));
//   - reflect: a call of reflect or internal/reflectlite on such a value,
//     grouped by what it reads (type name, kind, length, method set,
//     comparability, the rest);
//   - panic: a panic whose value or message depends on the type: a type
//     assertion without ok (*runtime.TypeAssertionError), a map key or a
//     comparison of two such values ("hash of unhashable type", "comparing
//     uncomparable type"), an explicit panic whose message has %T or a
//     reflect type name.
//
// Not in the inventory, on purpose: package reflect (the plugin's reflect
// is the VM's own, vm/vmreflect.go and the twin), generic functions (they
// have no native instance for the types of a plugin: the VM interprets them
// or implements them, vm/generics.go), the packages that plugins cannot
// import (vm/restrict.go, cmd/stdsyms), and dynamic calls of the methods of
// a caller's value (they always reach the plugin's method).
//
// Usage, once per Go release of the support window (internal/gorelease):
//
//	go run ./cmd/boundaryscan                      # writes docs/boundary/go1_NN.tsv and docs/BOUNDARY.md
//	GOTOOLCHAIN=go1.27.3 go run ./cmd/boundaryscan # the same for another release
//	go run ./cmd/boundaryscan -md                  # only docs/BOUNDARY.md, from the .tsv files
//	go run ./cmd/boundaryscan -uncovered io,fmt    # the rows of io and fmt without a probe
//
// docs/BOUNDARY.md merges the .tsv files of the releases and names the
// test program of each row (testdata/boundary_*.go, // row: lines).
// TestBoundaryInventory (cmd/runtests) fails when a row has no test, and
// when the files are not up to date.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/traefik/yaegi/v2/internal/boundary"
	"github.com/traefik/yaegi/v2/internal/gorelease"
)

func main() {
	out := flag.String("o", "docs", "output directory (docs/)")
	mdOnly := flag.Bool("md", false, "only write BOUNDARY.md from the .tsv files")
	uncovered := flag.String("uncovered", "", "only print the rows of these packages (comma-separated, or all) that have no probe, and the row comments that match no row")
	flag.Parse()
	if *uncovered != "" {
		n, err := boundary.Uncovered(*out, *uncovered, os.Stdout)
		if err != nil {
			fatal(err)
		}
		if n > 0 {
			os.Exit(1)
		}
		return
	}
	if !*mdOnly {
		rows, err := boundary.Scan()
		if err != nil {
			fatal(err)
		}
		minor, ok := gorelease.Minor(runtime.Version())
		if !ok {
			fatal(fmt.Errorf("not a Go release: %s", runtime.Version()))
		}
		prefix := fmt.Sprintf("go1_%d", minor)
		if err := boundary.WriteTSV(filepath.Join(*out, "boundary", prefix+".tsv"), rows); err != nil {
			fatal(err)
		}
		fmt.Printf("boundaryscan: %d rows for %s\n", len(rows), runtime.Version())
	}
	if err := boundary.WriteMD(*out); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "boundaryscan:", err)
	os.Exit(1)
}
