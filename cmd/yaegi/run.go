package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/syscall"
	"github.com/traefik/yaegi/stdlib/unrestricted"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

func run(arg []string) error {
	var interactive bool
	var tags string
	var cmd string
	var err error

	// The following flags are initialized from environment.
	useSyscall, _ := strconv.ParseBool(os.Getenv("YAEGI_SYSCALL"))
	useUnrestricted, _ := strconv.ParseBool(os.Getenv("YAEGI_UNRESTRICTED"))
	useUnsafe, _ := strconv.ParseBool(os.Getenv("YAEGI_UNSAFE"))

	rflag := flag.NewFlagSet("run", flag.ContinueOnError)
	rflag.BoolVar(&interactive, "i", false, "start an interactive REPL")
	rflag.BoolVar(&useSyscall, "syscall", useSyscall, "include syscall symbols")
	rflag.BoolVar(&useUnrestricted, "unrestricted", useUnrestricted, "include unrestricted symbols")
	rflag.StringVar(&tags, "tags", "", "set a list of build tags")
	rflag.BoolVar(&useUnsafe, "unsafe", useUnsafe, "include unsafe symbols")
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
		// Using a environment var allows a nested interpreter to import the syscall package.
		if err := os.Setenv("YAEGI_SYSCALL", "1"); err != nil {
			return err
		}
	}
	if useUnsafe {
		i.Use(unsafe.Symbols)
		if err := os.Setenv("YAEGI_UNSAFE", "1"); err != nil {
			return err
		}
	}
	if useUnrestricted {
		// Use of unrestricted symbols should always follow stdlib and syscall symbols, to update them.
		i.Use(unrestricted.Symbols)
		if err := os.Setenv("YAEGI_UNRESTRICTED", "1"); err != nil {
			return err
		}
	}

	if cmd != "" {
		_, err = i.Eval(cmd)
	}

	if len(args) == 0 {
		if interactive || cmd == "" {
			showError(err)
			_, err = i.REPL()
		}
		return err
	}

	// Skip first os arg to set command line as expected by interpreted main
	path := args[0]
	os.Args = arg
	flag.CommandLine = flag.NewFlagSet(path, flag.ExitOnError)

	if isFile(path) {
		err = runFile(i, path)
	} else {
		_, err = i.EvalPath(path)
	}

	if err != nil {
		return err
	}

	if interactive {
		_, err = i.REPL()
	}
	return err
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func runFile(i *interp.Interpreter, path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	if s := string(b); strings.HasPrefix(s, "#!") {
		// Allow executable go scripts, Have the same behavior as in interactive mode.
		s = strings.Replace(s, "#!", "//", 1)
		_, err = i.Eval(s)
		return err
	}

	// Files not starting with "#!" are supposed to be pure Go, directly Evaled.
	_, err = i.EvalPath(path)
	return err
}

func showError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	if p, ok := err.(interp.Panic); ok {
		fmt.Fprintln(os.Stderr, string(p.Stack))
	}
}
