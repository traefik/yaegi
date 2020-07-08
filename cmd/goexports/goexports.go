//go:generate go build

/*
Goexports generates wrappers of package exported symbols.

Output files are written in the current directory, and prefixed with the go version.

Usage:

    goexports package...

Example:

    goexports github.com/containous/yaegi/interp

The same goexport program is used for all target operating systems and architectures.
The GOOS and GOARCH environment variables set the desired target.
*/
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/containous/yaegi/extract"
)

// genLicense generates the correct LICENSE header text from the provided
// path to a LICENSE file.
func genLicense(fname string) (string, error) {
	if fname == "" {
		return "", nil
	}

	f, err := os.Open(fname)
	if err != nil {
		return "", fmt.Errorf("could not open LICENSE file: %v", err)
	}
	defer func() { _ = f.Close() }()

	license := new(strings.Builder)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		txt := sc.Text()
		if txt != "" {
			txt = " " + txt
		}
		license.WriteString("//" + txt + "\n")
	}
	if sc.Err() != nil {
		return "", fmt.Errorf("could not scan LICENSE file: %v", err)
	}

	return license.String(), nil
}

func main() {
	licenseFlag := flag.String("license", "", "path to a LICENSE file")

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		log.Fatalf("missing package path")
	}

	license, err := genLicense(*licenseFlag)
	if err != nil {
		log.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	ext := extract.Extractor{
		WorkingDir: wd,
		Dest:       path.Base(wd),
		License:    license,
	}
	goos, goarch := os.Getenv("GOOS"), os.Getenv("GOARCH")
	skip := map[string]bool{}
	if goos == "solaris" {
		skip["syscall.RawSyscall6"] = true
		skip["syscall.Syscall6"] = true
	}
	ext.Skip = skip

	for _, pkgIdent := range flag.Args() {
		var buf bytes.Buffer
		importPath, err := ext.Extract(pkgIdent, &buf)
		if err != nil {
			log.Println(err)
			continue
		}

		var oFile string
		if pkgIdent == "syscall" {
			oFile = strings.Replace(importPath, "/", "_", -1) + "_" + goos + "_" + goarch + ".go"
		} else {
			oFile = strings.Replace(importPath, "/", "_", -1) + ".go"
		}

		prefix := runtime.Version()
		if runtime.Version() != "devel" {
			parts := strings.Split(runtime.Version(), ".")

			prefix = parts[0] + "_" + extract.GetMinor(parts[1])
		}

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
