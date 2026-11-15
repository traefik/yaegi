package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// The init graph (initTasks) is read with go tool nm from the object file
// of each package, as the compiler writes it. Up to Go 1.27, the export
// file of go list -export is that object file (an archive). Go 1.28 turns
// on GOEXPERIMENT=golistexportnewformat: go list -export then gives the
// export data only, written by cmd/export, without symbols, and nm fails
// on it ("nm archive/tar: exit status 1" in the next Go job on tip). Then
// the packages are compiled again with go build -a -toolexec, where this
// command is the toolexec program: after each run of the compiler, it runs
// nm on the object file that the compiler wrote (-linkobj on Go 1.28,
// compileOutputs), and keeps the output.

const (
	// toolexecEnv is the directory where the toolexec child writes the nm
	// output of each package (one file per package, named by the escaped
	// package path). When it is set, the command is that child.
	toolexecEnv = "YAEGI_STDEXPORT_TOOLEXEC"
	// nmEnv is the path of the go tool nm binary, for the child.
	nmEnv = "YAEGI_STDEXPORT_NM"
)

// objectSymbols returns the nm output of the object file of each package of
// pkgs (by package path), from the export files of go list -export when
// they are object files, else from go build -a -toolexec of patterns.
// Packages without Go files on the platform (runtime/cgo without cgo) and
// the packages that are not in the graph (unsafe, cmd/...) are not in the
// result.
func objectSymbols(nm string, env []string, pkgs []*packages.Package, patterns []string) (map[string]string, error) {
	var want []*packages.Package
	archives := true
	for _, p := range pkgs {
		if p.PkgPath == "unsafe" || strings.HasPrefix(p.PkgPath, "cmd/") || len(p.GoFiles) == 0 {
			continue
		}
		want = append(want, p)
		if p.ExportFile == "" {
			return nil, fmt.Errorf("%s: no export file", p.PkgPath)
		}
		if archives {
			ok, err := isArchive(p.ExportFile)
			if err != nil {
				return nil, err
			}
			archives = ok
		}
	}
	if archives {
		return archiveSymbols(nm, want)
	}
	return toolexecSymbols(nm, env, want, patterns)
}

// archiveSymbols runs nm on the export file of each package of want.
func archiveSymbols(nm string, want []*packages.Package) (map[string]string, error) {
	syms := map[string]string{}
	for _, p := range want {
		out, err := runNM(nm, p.ExportFile)
		if err != nil {
			return nil, fmt.Errorf("nm %s: %v", p.PkgPath, err)
		}
		syms[p.PkgPath] = out
	}
	return syms, nil
}

// toolexecSymbols compiles patterns again with go build -a -toolexec, this
// command being the toolexec child, and returns the nm output of the
// object file of each package of want.
func toolexecSymbols(nm string, env []string, want []*packages.Package, patterns []string) (map[string]string, error) {
	dir, err := os.MkdirTemp("", "stdexport-nm-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// -a: the compiler must run for each package, also when its object
	// file is in the build cache (the toolexec child only sees a run).
	args := append([]string{"build", "-a", "-toolexec", self}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(env[:len(env):len(env)], toolexecEnv+"="+dir, nmEnv+"="+nm)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	syms := map[string]string{}
	for _, p := range want {
		b, err := os.ReadFile(filepath.Join(dir, url.PathEscape(p.PkgPath)))
		if err != nil {
			return nil, fmt.Errorf("%s: no nm output from go build -toolexec: %v", p.PkgPath, err)
		}
		syms[p.PkgPath] = string(b)
	}
	return syms, nil
}

// isArchive reports whether file is an archive (an object file of the
// compiler), not export data alone.
func isArchive(file string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer f.Close()
	magic := make([]byte, 8)
	if _, err := io.ReadFull(f, magic); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && err != io.EOF {
		return false, err
	}
	return string(magic) == "!<arch>\n", nil
}

// runNM runs nm on file. The error has what nm wrote on its standard error.
func runNM(nm, file string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command(nm, file)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.HasSuffix(msg, ": no symbols") {
			// go tool nm fails on a file without symbols: export data
			// only, or an object file with nothing in it. Neither has an
			// init task.
			return "", nil
		}
		if msg != "" {
			return "", fmt.Errorf("%v: %s", err, msg)
		}
		return "", err
	}
	return string(out), nil
}

// compileOutputs returns the package path (-p) and the object file of a
// compile command line (the arguments after the tool). The object file is
// the one of -linkobj when it is set, else the one of -o. Up to Go 1.27,
// cmd/go runs "compile -o obj.a" and obj.a holds the export data and the
// object code. Go 1.28 runs "compile -o export.a -linkobj obj.a": export.a
// holds the export data only, without symbols, and nm must read obj.a.
// Both "-flag value" and "-flag=value" are read, as the flag package does.
func compileOutputs(args []string) (pkg, obj string) {
	var out, linkobj string
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			continue
		}
		// --flag is -flag
		name, val, hasVal := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(args[i], "-"), "-"), "=")
		var dst *string
		switch name {
		case "p":
			dst = &pkg
		case "o":
			dst = &out
		case "linkobj":
			dst = &linkobj
		default:
			continue
		}
		if !hasVal {
			if i+1 >= len(args) {
				break
			}
			i++
			val = args[i]
		}
		*dst = val
	}
	if linkobj != "" {
		return pkg, linkobj
	}
	return pkg, out
}

// toolexec is the toolexec child: it runs the tool of args, and after a
// compile of a package (flags -p and -o or -linkobj, compileOutputs) it
// writes the nm output of the object file to dir. It returns the exit code.
func toolexec(dir, nm string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "stdexport: toolexec without a tool")
		return 2
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() > 0 {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "stdexport: toolexec %s: %v\n", args[0], err)
		return 1
	}
	if strings.TrimSuffix(filepath.Base(args[0]), ".exe") != "compile" {
		return 0
	}
	pkg, obj := compileOutputs(args[1:])
	if pkg == "" || obj == "" {
		return 0 // compile -V=full: the tool ID
	}
	out, err := runNM(nm, obj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdexport: nm %s: %v\n", pkg, err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, url.PathEscape(pkg)), []byte(out), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "stdexport: %v\n", err)
		return 1
	}
	return 0
}
