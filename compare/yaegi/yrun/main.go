// Command yrun interprets a Go program with yaegi configured as Traefik does
// (pkg/plugins/middlewareyaegi.go): stdlib.Symbols, restricted mode, and
// unsafe+syscall symbols only with -unsafe (Traefik's useUnsafe setting).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/syscall"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

func main() {
	useUnsafe := flag.Bool("unsafe", true, "load unsafe and syscall symbols (Traefik useUnsafe)")
	gopath := flag.String("gopath", "", "GOPATH to use (default: built from go list -deps)")
	timing := flag.Bool("t", false, "print load time to stderr")
	flag.Parse()
	file := flag.Arg(0)
	start := time.Now()
	gp := *gopath
	if gp == "" {
		var err error
		gp, err = os.MkdirTemp("", "yrun-gopath-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
		defer os.RemoveAll(gp)
		if err := linkGopath(gp, file); err != nil {
			fmt.Fprintln(os.Stderr, "gopath:", err)
			os.Exit(4)
		}
	}
	i := interp.New(interp.Options{GoPath: gp, Env: os.Environ(), Stdout: os.Stdout, Stderr: os.Stderr})
	must(i.Use(stdlib.Symbols))
	if *useUnsafe {
		must(i.Use(unsafe.Symbols))
		must(i.Use(syscall.Symbols))
	}
	_, err := i.EvalPath(file)
	if *timing {
		fmt.Fprintf(os.Stderr, "yaegi total %v\n", time.Since(start))
	}
	if err != nil {
		var p interp.Panic
		if errors.As(err, &p) {
			fmt.Fprintln(os.Stderr, "panic:", p.Value)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
}

func linkGopath(gopath, file string) error {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}} {{.Dir}}{{end}}", file)
	cmd.Dir = filepath.Dir(file)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	var pairs [][2]string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] != "command-line-arguments" {
			pairs = append(pairs, [2]string{f[0], f[1]})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return len(pairs[i][0]) < len(pairs[j][0]) })
	for _, p := range pairs {
		dst := filepath.Join(gopath, "src", filepath.FromSlash(p[0]))
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(p[1], dst); err != nil {
			return err
		}
	}
	return nil
}
