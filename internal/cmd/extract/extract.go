//go:generate go build

/*
extract generates wrappers of stdlib package exported symbols. This command
is reserved for internal use in yaegi project.

For a similar purpose with third party packages, see the yaegi extract subcommand,
based on the same code.

Output files are written in the current directory, and prefixed with the go version.

Usage:

	extract package...

The same program is used for all target operating systems and architectures.
The GOOS and GOARCH environment variables set the desired target.
*/
package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/traefik/yaegi/extract"
)

var (
	exclude = flag.String("exclude", "", "comma separated list of regexp matching symbols to exclude")
	include = flag.String("include", "", "comma separated list of regexp matching symbols to include")
)

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		log.Fatalf("missing package path")
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	ext := extract.Extractor{
		Dest: path.Base(wd),
	}

	goos, goarch := os.Getenv("GOOS"), os.Getenv("GOARCH")

	if *exclude != "" {
		ext.Exclude = strings.Split(*exclude, ",")
	}

	if *include != "" {
		ext.Include = strings.Split(*include, ",")
	}

	for _, pkgIdent := range flag.Args() {
		var buf bytes.Buffer

		if pkgIdent == "syscall" && goos == "solaris" {
			// Syscall6 is broken on solaris (https://github.com/golang/go/issues/24357),
			// it breaks build, skip related symbols.
			ext.Exclude = append(ext.Exclude, "Syscall6")
		}

		importPath, err := ext.Extract(pkgIdent, "", &buf)
		if err != nil {
			log.Fatal(err)
		}

		var oFile string
		if pkgIdent == "syscall" {
			oFile = strings.ReplaceAll(importPath, "/", "_") + "_" + goos + "_" + goarch + ".go"
		} else {
			oFile = strings.ReplaceAll(importPath, "/", "_") + ".go"
		}

		version := runtime.Version()
		if strings.HasPrefix(version, "devel") {
			log.Fatalf("extracting only supported with stable releases of Go, not %v", version)
		}
		parts := strings.Split(version, ".")
		prefix := parts[0] + "_" + extract.GetMinor(parts[1])

		f, err := os.Create(prefix + "_" + oFile)
		if err != nil {
			log.Fatal(err)
		}

		if _, err := io.Copy(f, &buf); err != nil {
			_ = f.Close()
			log.Fatal(err)
		}

		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
