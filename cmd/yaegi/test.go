package main

import (
	"errors"
	"flag"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/syscall"
	"github.com/traefik/yaegi/stdlib/unrestricted"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

func test(arg []string) (err error) {
	var (
		bench     string
		benchmem  bool
		benchtime string
		count     string
		cpu       string
		failfast  bool
		run       string
		short     bool
		tags      string
		timeout   string
		verbose   bool
	)

	// The following flags are initialized from environment.
	useSyscall, _ := strconv.ParseBool(os.Getenv("YAEGI_SYSCALL"))
	useUnrestricted, _ := strconv.ParseBool(os.Getenv("YAEGI_UNRESTRICTED"))
	useUnsafe, _ := strconv.ParseBool(os.Getenv("YAEGI_UNSAFE"))

	tflag := flag.NewFlagSet("test", flag.ContinueOnError)
	tflag.StringVar(&bench, "bench", "", "Run only those benchmarks matching a regular expression.")
	tflag.BoolVar(&benchmem, "benchmem", false, "Print memory allocation statistics for benchmarks.")
	tflag.StringVar(&benchtime, "benchtime", "", "Run enough iterations of each benchmark to take t.")
	tflag.StringVar(&count, "count", "", "Run each test and benchmark n times (default 1).")
	tflag.StringVar(&cpu, "cpu", "", "Specify a list of GOMAXPROCS values for which the tests or benchmarks should be executed.")
	tflag.BoolVar(&failfast, "failfast", false, "Do not start new tests after the first test failure.")
	tflag.StringVar(&run, "run", "", "Run only those tests matching a regular expression.")
	tflag.BoolVar(&short, "short", false, "Tell long-running tests to shorten their run time.")
	tflag.StringVar(&tags, "tags", "", "Set a list of build tags.")
	tflag.StringVar(&timeout, "timeout", "", "If a test binary runs longer than duration d, panic.")
	tflag.BoolVar(&useUnrestricted, "unrestricted", useUnrestricted, "Include unrestricted symbols.")
	tflag.BoolVar(&useUnsafe, "unsafe", useUnsafe, "Include usafe symbols.")
	tflag.BoolVar(&useSyscall, "syscall", useSyscall, "Include syscall symbols.")
	tflag.BoolVar(&verbose, "v", false, "Verbose output: log all tests as they are run.")
	tflag.Usage = func() {
		fmt.Println("Usage: yaegi test [options] [path]")
		fmt.Println("Options:")
		tflag.PrintDefaults()
	}
	if err = tflag.Parse(arg); err != nil {
		return err
	}
	args := tflag.Args()
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Overwrite os.Args with correct flags to setup testing.Init.
	tf := []string{""}
	if bench != "" {
		tf = append(tf, "-test.bench", bench)
	}
	if benchmem {
		tf = append(tf, "-test.benchmem")
	}
	if benchtime != "" {
		tf = append(tf, "-test.benchtime", benchtime)
	}
	if count != "" {
		tf = append(tf, "-test.count", count)
	}
	if cpu != "" {
		tf = append(tf, "-test.cpu", cpu)
	}
	if failfast {
		tf = append(tf, "-test.failfast")
	}
	if run != "" {
		tf = append(tf, "-test.run", run)
	}
	if short {
		tf = append(tf, "-test.short")
	}
	if timeout != "" {
		tf = append(tf, "-test.timeout", timeout)
	}
	if verbose {
		tf = append(tf, "-test.v")
	}
	testing.Init()
	os.Args = tf
	flag.Parse()
	path += string(filepath.Separator)
	var dir string

	switch strings.Split(path, string(filepath.Separator))[0] {
	case ".", "..", string(filepath.Separator):
		dir = path
	default:
		dir = filepath.Join(build.Default.GOPATH, "src", path)
	}
	if err = os.Chdir(dir); err != nil {
		return err
	}

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
	if useUnrestricted {
		if err := i.Use(unrestricted.Symbols); err != nil {
			return err
		}
		if err := os.Setenv("YAEGI_UNRESTRICTED", "1"); err != nil {
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
	if err = i.EvalTest(path); err != nil {
		return err
	}

	benchmarks := []testing.InternalBenchmark{}
	tests := []testing.InternalTest{}
	syms, ok := i.Symbols(path)[path]
	if !ok {
		return errors.New("No tests found")
	}
	for name, sym := range syms {
		switch fun := sym.Interface().(type) {
		case func(*testing.B):
			benchmarks = append(benchmarks, testing.InternalBenchmark{name, fun})
		case func(*testing.T):
			tests = append(tests, testing.InternalTest{name, fun})
		}
	}

	testing.Main(regexp.MatchString, tests, benchmarks, nil)
	return nil
}
