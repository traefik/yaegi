package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"strings"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
	"github.com/containous/yaegi/stdlib/syscall"
	"github.com/containous/yaegi/stdlib/unrestricted"
	"github.com/containous/yaegi/stdlib/unsafe"
)

func run(arg []string) error {
	var interactive bool
	var useSyscall bool
	var useUnrestricted bool
	var useUnsafe bool
	var tags string
	var cmd string
	var err error

	rflag := flag.NewFlagSet("run", flag.ContinueOnError)
	rflag.BoolVar(&interactive, "i", false, "start an interactive REPL")
	rflag.BoolVar(&useSyscall, "syscall", false, "include syscall symbols")
	rflag.BoolVar(&useUnrestricted, "unrestricted", false, "include unrestricted symbols")
	rflag.StringVar(&tags, "tags", "", "set a list of build tags")
	rflag.BoolVar(&useUnsafe, "unsafe", false, "include usafe symbols")
	rflag.StringVar(&cmd, "e", "", "set the command to be executed (instead of script or/and shell)")
	rflag.Usage = func() {
		fmt.Println("Usage: yaegi run [options] [path] [args]")
		fmt.Println("Options:")
		rflag.PrintDefaults()
	}
	if err = rflag.Parse(arg); err != nil {
		return err
	}
	args := rflag.Args()

	i := interp.New(interp.Options{GoPath: build.Default.GOPATH, BuildTags: strings.Split(tags, ",")})
	i.Use(stdlib.Symbols)
	i.Use(interp.Symbols)
	if useSyscall {
		i.Use(syscall.Symbols)
	}
	if useUnsafe {
		i.Use(unsafe.Symbols)
	}
	if useUnrestricted {
		// Use of unrestricted symbols should always follow use of stdlib symbols, to update them.
		i.Use(unrestricted.Symbols)
	}

	if cmd != "" {
		i.REPL(strings.NewReader(cmd), os.Stderr)
	}

	if len(args) == 0 {
		if interactive || cmd == "" {
			i.REPL(os.Stdin, os.Stdout)
		}
		return nil
	}

	// Skip first os arg to set command line as expected by interpreted main
	path := args[0]
	os.Args = arg[1:]
	flag.CommandLine = flag.NewFlagSet(path, flag.ExitOnError)

	if isPackageName(path) {
		err = runPackage(i, path)
	} else {
		if isDir(path) {
			err = runDir(i, path)
		} else {
			err = runFile(i, path)
		}
	}
	if err != nil {
		return err
	}

	if interactive {
		i.REPL(os.Stdin, os.Stdout)
	}
	return nil
}

func isPackageName(path string) bool {
	return !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "../") && !strings.HasSuffix(path, ".go")
}

func isDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir()
}

func runPackage(i *interp.Interpreter, path string) error {
	return fmt.Errorf("runPackage not implemented")
}

func runDir(i *interp.Interpreter, path string) error {
	return fmt.Errorf("runDir not implemented")
}

func runFile(i *interp.Interpreter, path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	if s := string(b); strings.HasPrefix(s, "#!") {
		// Allow executable go scripts, Have the same behavior as in interactive mode.
		s = strings.Replace(s, "#!", "//", 1)
		i.REPL(strings.NewReader(s), os.Stdout)
	} else {
		// Files not starting with "#!" are supposed to be pure Go, directly Evaled.
		_, err := i.EvalPath(path)
		if err != nil {
			fmt.Println(err)
			if p, ok := err.(interp.Panic); ok {
				fmt.Println(string(p.Stack))
			}
		}
	}
	return nil
}
