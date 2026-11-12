// Command vmhost runs a test program made by cmd/stdtests with yaegi v2: the
// test files are interpreted, the package under test is the native one (VM
// mode) or its AOT copy (-aot), and package testmain of cmd/stdtests is
// native (see its doc).
//
// Usage: vmhost [-aot] <program dir> [test flags...]
//
// The program runs in the current directory (the directory of the package
// under test in GOROOT, for its testdata/ files). Exit codes are the ones
// of yaegi: 0 success, 1 failed tests (os.Exit of the program), 2 unrecovered
// panic, 3 construct not supported by the VM, 4 load or type error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"

	_ "github.com/traefik/yaegi/v2/aot/all" // AOT copies (-aot)
	"github.com/traefik/yaegi/v2/cmd/stdtests/testmain"
	_ "github.com/traefik/yaegi/v2/stdsyms/sideeffect" // expvar, net/http/pprof and testing/quick, as with "go test"
	"github.com/traefik/yaegi/v2/vm"
)

const testmainPath = "github.com/traefik/yaegi/v2/cmd/stdtests/testmain"

func main() {
	flags := flag.NewFlagSet("vmhost", flag.ExitOnError)
	aot := flags.Bool("aot", false, "run the packages that have an AOT copy in AOT mode")
	_ = flags.Parse(os.Args[1:])
	if flags.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: vmhost [-aot] <program dir> [test flags...]")
		os.Exit(2)
	}
	dir := flags.Arg(0)
	os.Args = flags.Args()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	vm.RegisterNativePackage(&vm.Package{
		Path:  testmainPath,
		Funcs: map[string]reflect.Value{"Main": reflect.ValueOf(testmain.Main)},
	})
	opts := vm.RunOptions{
		Dir:   dir,
		AOT:   *aot,
		Modes: map[string]vm.Mode{testmainPath: vm.ModeNative},
	}
	_, err := vm.RunMain(opts, ".")
	var ge *vm.GoexitError
	if errors.As(err, &ge) {
		ge.Wait()
		fmt.Fprintln(os.Stderr, "fatal error: no goroutines (main called runtime.Goexit) - deadlock!")
		os.Exit(2)
	}
	var (
		le *vm.LoadError
		ce *vm.CompileError
		pe *vm.PanicError
	)
	switch {
	case err == nil:
		os.Exit(0)
	case errors.As(err, &ce):
		fmt.Fprintln(os.Stderr, "yaegi:", ce.Msg)
		os.Exit(3)
	case errors.As(err, &le):
		fmt.Fprintln(os.Stderr, le.Msg)
		os.Exit(4)
	case errors.As(err, &pe):
		fmt.Fprintln(os.Stderr, pe.Error())
		if pe.Trace != "" {
			fmt.Fprint(os.Stderr, "\n", pe.Trace)
		}
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
