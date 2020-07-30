package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/containous/yaegi/extract"
)

func extractCmd(arg []string) error {
	var licensePath string
	var importPath string

	eflag := flag.NewFlagSet("run", flag.ContinueOnError)
	eflag.StringVar(&licensePath, "license", "", "path to a LICENSE file")
	eflag.StringVar(&importPath, "import_path", "", "the namespace for the extracted symbols")
	eflag.Usage = func() {
		fmt.Println("Usage: yaegi extract [options] packages...")
		fmt.Println("Options:")
		eflag.PrintDefaults()
	}

	if err := eflag.Parse(arg); err != nil {
		return err
	}

	args := eflag.Args()
	if len(args) == 0 {
		return fmt.Errorf("missing package")
	}

	license, err := genLicense(licensePath)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	ext := extract.Extractor{
		Dest:    path.Base(wd),
		License: license,
	}

	for _, pkgIdent := range args {
		var buf bytes.Buffer
		importPath, err := ext.Extract(pkgIdent, importPath, &buf)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		oFile := strings.Replace(importPath, "/", "_", -1) + ".go"
		f, err := os.Create(oFile)
		if err != nil {
			return err
		}

		if _, err := io.Copy(f, &buf); err != nil {
			_ = f.Close()
			return err
		}

		if err := f.Close(); err != nil {
			return err
		}
	}

	return nil
}

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
