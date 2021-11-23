package main

import (
	"flag"
	"fmt"
	"go/build"
	"os"
	"reflect"
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
	var noAutoImport bool
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
	rflag.BoolVar(&noAutoImport, "noautoimport", false, "do not auto import pre-compiled packages. Import names that would result in collisions (e.g. rand from crypto/rand and rand from math/rand) are automatically renamed (crypto_rand and math_rand)")
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

	i := interp.New(interp.Options{
		GoPath:       build.Default.GOPATH,
		BuildTags:    strings.Split(tags, ","),
		Env:          os.Environ(),
		Unrestricted: useUnrestricted,
	})
	if err := i.Use(stdlib.Symbols); err != nil {
		return err
	}
	if err := i.Use(interp.Symbols); err != nil {
		return err
	}
	if useSyscall {
		if err := i.Use(syscall.Symbols); err != nil {
			return err
		}
		// Using a environment var allows a nested interpreter to import the syscall package.
		if err := os.Setenv("YAEGI_SYSCALL", "1"); err != nil {
			return err
		}
	}
	if useUnsafe {
		if err := i.Use(unsafe.Symbols); err != nil {
			return err
		}
		if err := os.Setenv("YAEGI_UNSAFE", "1"); err != nil {
			return err
		}
	}
	if useUnrestricted {
		// Use of unrestricted symbols should always follow stdlib and syscall symbols, to update them.
		if err := i.Use(unrestricted.Symbols); err != nil {
			return err
		}
		if err := os.Setenv("YAEGI_UNRESTRICTED", "1"); err != nil {
			return err
		}
	}

	if cmd != "" {
		if !noAutoImport {
			i.ImportUsed()
		}
		var v reflect.Value
		v, err = i.Eval(cmd)
		if len(args) == 0 && v.IsValid() {
			fmt.Println(v)
		}
	}

	if len(args) == 0 {
		if cmd == "" || interactive {
			showError(err)
			if !noAutoImport {
				i.ImportUsed()
			}
			_, err = i.REPL()
		}
		return err
	}

	// Skip first os arg to set command line as expected by interpreted main.
	path := args[0]
	os.Args = arg
	flag.CommandLine = flag.NewFlagSet(path, flag.ExitOnError)

	if isFile(path) {
		err = runFile(i, path, noAutoImport)
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

func runFile(i *interp.Interpreter, path string, noAutoImport bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if s := string(b); strings.HasPrefix(s, "#!") {
		// Allow executable go scripts, Have the same behavior as in interactive mode.
		s = strings.Replace(s, "#!", "//", 1)
		if !noAutoImport {
			i.ImportUsed()
		}
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
