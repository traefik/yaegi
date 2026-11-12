package vm

// Ways to end the host process that do not go through the os.Exit and
// log.Fatal of the plugin's symbol table (stdsyms/restricted.go). A
// restricted machine (not m.realStd) replaces them:
//
//   - Native code that calls os.Exit itself. (*flag.FlagSet).Parse with
//     flag.ExitOnError calls os.Exit(2), or os.Exit(0) for -h: for the
//     machine it calls the os.Exit of the plugin instead, which panics with
//     "os.Exit(2)" like a call from the plugin. The Fatal methods of a
//     native *log.Logger (slog.NewLogLogger returns one) panic like those of
//     the plugin's loggers.
//   - The global state of package flag. flag.Parse parses os.Args into
//     flag.CommandLine, and flag.Bool and the other functions define flags
//     in flag.CommandLine: the host's. A restricted machine has its own
//     flag.CommandLine and flag.Usage, and its own copy of os.Args, so the
//     plugin can neither parse nor change the flags and arguments of the
//     host.
//   - The AOT copies (cmd/aotgen). aotgen rewrites their calls to os.Exit
//     and log.Fatal, and their reads of os.Args, into AOTExit, AOTFatal and AOTArgs
//     below: they follow the machine of the goroutine.
//
// The native methods are replaced for static calls, method values and
// calls through an interface (lookupMethod), and for the methods that the
// plugin gets through its reflect (the twin: Value.Method,
// Value.MethodByName, Type.Method, Type.MethodByName).
//
// Native code can still call a native method through the native reflect:
// text/template and html/template call the methods of their data. So a
// native *flag.FlagSet of a restricted machine never holds
// flag.ExitOnError: flag.NewFlagSet and (*flag.FlagSet).Init give it
// flag.ContinueOnError, and keep the mode that the plugin asked for in
// exitSets. A native Parse reached by native code then returns the error
// instead of ending the process. The Fatal methods of a native *log.Logger
// return nothing: text/template and html/template refuse to call them.
//
// TestFlagExit and TestFlagCommandLine check these replacements.

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"runtime"
	"sync"
	"weak"

	"github.com/traefik/yaegi/v2/stdsyms"
)

// osExitOf returns the os.Exit of the machine's programs: the restricted
// one (it panics) unless m.realStd, and the override of RunOptions.BeforeExit
// if any.
func (m *Machine) osExitOf() func(int) {
	if rv, ok := m.overrides["os.Exit"]; ok {
		return rv.Interface().(func(int))
	}
	syms := stdsyms.Symbols["os/os"]
	if m.realStd {
		syms = unrestrictedSyms()["os/os"]
	}
	return syms["Exit"].Interface().(func(int))
}

// restrictedExit is the os.Exit of restricted machines.
var restrictedExit = sync.OnceValue(func() func(int) {
	return stdsyms.Symbols["os/os"]["Exit"].Interface().(func(int))
})

// restrictedMethods replace native methods for the restricted machines
// (methodFunc: the receiver comes first).
var restrictedMethods = sync.OnceValue(func() map[methodKey]reflect.Value {
	fs := reflect.TypeOf((*flag.FlagSet)(nil))
	lg := reflect.TypeOf((*log.Logger)(nil))
	return map[methodKey]reflect.Value{
		{fs, "Parse"}:         reflect.ValueOf(restrictedParse),
		{fs, "Init"}:          reflect.ValueOf(restrictedInit),
		{fs, "ErrorHandling"}: reflect.ValueOf(restrictedErrorHandling),
		{lg, "Fatal"}:         reflect.ValueOf((*log.Logger).Panic),
		{lg, "Fatalf"}:        reflect.ValueOf((*log.Logger).Panicf),
		{lg, "Fatalln"}:       reflect.ValueOf((*log.Logger).Panicln),
	}
})

// exitSets holds the native flag sets of restricted machines for which the
// plugin asked flag.ExitOnError (NewFlagSet, Init): their native mode is
// flag.ContinueOnError. The keys are weak: an entry goes away with its flag
// set.
var exitSets sync.Map // weak.Pointer[flag.FlagSet] -> bool

// nativeMode is the mode that a native flag set of a restricted machine
// gets for the mode h that the plugin asked for.
func nativeMode(h flag.ErrorHandling) flag.ErrorHandling {
	if h == flag.ExitOnError {
		return flag.ContinueOnError
	}
	return h
}

// setAskedMode records the mode h that the plugin asked for f.
func setAskedMode(f *flag.FlagSet, h flag.ErrorHandling) {
	k := weak.Make(f)
	if _, loaded := exitSets.Swap(k, h == flag.ExitOnError); !loaded {
		runtime.AddCleanup(f, func(k weak.Pointer[flag.FlagSet]) { exitSets.Delete(k) }, k)
	}
}

// askedExit reports whether the plugin asked flag.ExitOnError for f.
func askedExit(f *flag.FlagSet) bool {
	v, ok := exitSets.Load(weak.Make(f))
	return ok && v.(bool)
}

// restrictedNewFlagSet is flag.NewFlagSet for the restricted machines.
func restrictedNewFlagSet(name string, h flag.ErrorHandling) *flag.FlagSet {
	f := flag.NewFlagSet(name, nativeMode(h))
	setAskedMode(f, h)
	return f
}

// restrictedInit is (*flag.FlagSet).Init for the restricted machines.
func restrictedInit(f *flag.FlagSet, name string, h flag.ErrorHandling) {
	f.Init(name, nativeMode(h))
	setAskedMode(f, h)
}

// restrictedErrorHandling is (*flag.FlagSet).ErrorHandling for the
// restricted machines: the mode that the plugin asked for.
func restrictedErrorHandling(f *flag.FlagSet) flag.ErrorHandling {
	if askedExit(f) {
		return flag.ExitOnError
	}
	return f.ErrorHandling()
}

// restrictedParse is (*flag.FlagSet).Parse that does not end the process.
// With flag.ExitOnError, the flag set parses with ContinueOnError (the
// messages and the usage are printed the same way), then the restricted
// os.Exit gets the code flag would give to os.Exit: 0 for -h and -help,
// 2 for the other errors.
func restrictedParse(f *flag.FlagSet, args []string) error {
	if f.ErrorHandling() == flag.ExitOnError {
		// Not set by restrictedInit or restrictedNewFlagSet (a flag set
		// made by native code): from now on, keep the mode in exitSets.
		restrictedInit(f, f.Name(), flag.ExitOnError)
	}
	if !askedExit(f) {
		return f.Parse(args)
	}
	err := f.Parse(args)
	if err == nil {
		return nil
	}
	if err == flag.ErrHelp {
		restrictedExit()(0)
	} else {
		restrictedExit()(2)
	}
	return err
}

// restrictedMethodOf returns the replacement of the method name of the
// native type t (restrictedMethods) when the goroutine runs a restricted
// machine, or no machine, the refused methods (refusedmeth.go), and the
// RawConn methods that lend their file descriptor (rawconn.go).
func restrictedMethodOf(t reflect.Type, name string) (reflect.Value, bool) {
	m := curMachine()
	if fn, ok := m.fatalMethod(t, name); ok {
		return fn, true
	}
	if fn, ok := refusedMethodOf(m, t, name); ok {
		return fn, true
	}
	if fn, ok := execMethodOf(m, t, name); ok {
		return fn, true
	}
	if fn, ok := rawConnMethodOf(m, t, name); ok {
		return fn, true
	}
	if m != nil && m.realStd {
		return reflect.Value{}, false
	}
	if m != nil {
		if fn, ok := m.hostMethod(t, name); ok {
			return fn, true
		}
	}
	fn, ok := restrictedMethods()[methodKey{t, name}]
	return fn, ok
}

// restrictedMethodValue returns mv, the method name of the native value v
// for the twin of reflect (Value.Method, Value.MethodByName), or a function
// that calls its replacement on v.
func restrictedMethodValue(v reflect.Value, name string, mv reflect.Value) reflect.Value {
	if !mv.IsValid() {
		return mv
	}
	recv := v
	if recv.Kind() == reflect.Interface {
		recv = recv.Elem()
	}
	fn, ok := restrictedMethodOf(recv.Type(), name)
	if !ok {
		return chanReaderFunc(recv.Type(), name, mv)
	}
	ft := mv.Type()
	return chanReaderFunc(recv.Type(), name, reflect.MakeFunc(ft, func(in []reflect.Value) []reflect.Value {
		in = append([]reflect.Value{recv}, in...)
		if ft.IsVariadic() {
			return fn.CallSlice(in)
		}
		return fn.Call(in)
	}))
}

// restrictedMethodFunc returns meth, a method of the native type t for the
// twin of reflect (Type.Method, Type.MethodByName), with the function of
// its replacement.
func restrictedMethodFunc(t reflect.Type, meth reflect.Method) reflect.Method {
	if !meth.Func.IsValid() {
		return meth
	}
	if fn, ok := restrictedMethodOf(t, meth.Name); ok {
		meth.Func = fn
	}
	meth.Func = chanReaderFunc(t, meth.Name, meth.Func)
	return meth
}

// flagState is the global state of package flag for one restricted machine.
type flagState struct {
	args        []string      // os.Args
	commandLine *flag.FlagSet // flag.CommandLine
	usage       func()        // flag.Usage
	funcs       map[string]reflect.Value
}

// flags returns the flag state of m, made at the first use: a copy of the
// host's os.Args, and a flag.CommandLine set up as in package flag (init).
func (m *Machine) flags() *flagState {
	m.flagOnce.Do(func() {
		st := &flagState{args: append([]string(nil), os.Args...)}
		name := ""
		if len(st.args) > 0 {
			name = st.args[0]
		}
		st.commandLine = restrictedNewFlagSet(name, flag.ExitOnError)
		st.commandLine.Usage = func() { st.usage() } // flag's commandLineUsage
		st.usage = func() {
			fmt.Fprintf(st.commandLine.Output(), "Usage of %s:\n", st.args[0])
			st.commandLine.PrintDefaults()
		}
		st.funcs = flagFuncs(st)
		m.flagSt = st
	})
	return m.flagSt
}

// flagPure are the functions of package flag that do not use
// flag.CommandLine.
var flagPure = map[string]bool{"NewFlagSet": true, "UnquoteUsage": true}

// flagFuncs returns the functions of package flag that use
// flag.CommandLine, on st: each one calls the method of the same name on
// the current value of st.commandLine, as package flag does.
func flagFuncs(st *flagState) map[string]reflect.Value {
	fs := reflect.TypeOf((*flag.FlagSet)(nil))
	funcs := map[string]reflect.Value{
		"Parse": reflect.ValueOf(func() {
			// Errors are ignored: CommandLine is set for ExitOnError.
			restrictedParse(st.commandLine, st.args[1:])
		}),
	}
	for name, rv := range stdsyms.Symbols["flag/flag"] {
		if rv.Kind() != reflect.Func || flagPure[name] || funcs[name].IsValid() {
			continue
		}
		meth, ok := fs.MethodByName(name)
		if !ok {
			continue // flagFunc refuses it
		}
		ft := rv.Type()
		funcs[name] = reflect.MakeFunc(ft, func(in []reflect.Value) []reflect.Value {
			in = append([]reflect.Value{reflect.ValueOf(st.commandLine)}, in...)
			if ft.IsVariadic() {
				return meth.Func.CallSlice(in)
			}
			return meth.Func.Call(in)
		})
	}
	return funcs
}

// flagFunc returns the function name of package flag for the restricted
// machine m, when it uses flag.CommandLine. A function of package flag that
// is neither pure nor a method of FlagSet (a new Go version) is refused:
// it could reach the host's CommandLine.
func (m *Machine) flagFunc(name string) (reflect.Value, bool) {
	if m.realStd || m.modeOf("flag") != ModeNative {
		return reflect.Value{}, false
	}
	if name == "NewFlagSet" {
		return reflect.ValueOf(restrictedNewFlagSet), true
	}
	if flagPure[name] {
		return reflect.Value{}, false
	}
	if rv, ok := m.flags().funcs[name]; ok {
		return rv, true
	}
	unsupported("flag.%s is not available to restricted programs", name)
	return reflect.Value{}, false
}

// restrictedVar returns the address of the variable pkg.name for the
// restricted machine m, when it has its own copy: os.Args,
// flag.CommandLine and flag.Usage, and the registries of hostregs.go.
func (m *Machine) restrictedVar(pkg, name string) (reflect.Value, bool) {
	if m.realStd {
		return reflect.Value{}, false
	}
	if rv, ok := m.hostVar(pkg, name); ok {
		return rv, true
	}
	switch {
	case pkg == "os" && name == "Args":
		return reflect.ValueOf(&m.flags().args), true
	case pkg == "flag" && m.modeOf("flag") == ModeNative:
		switch name {
		case "CommandLine":
			return reflect.ValueOf(&m.flags().commandLine), true
		case "Usage":
			return reflect.ValueOf(&m.flags().usage), true
		}
	}
	return reflect.Value{}, false
}

// lookupMethod is lookupMethod for m: the restricted machines get the
// methods of restrictedMethods, the replacements of the refused methods
// (refusedmeth.go), and with the syscall subset the RawConn methods that
// lend their file descriptor (rawconn.go).
func (m *Machine) lookupMethod(t reflect.Type, name string) *methodFunc {
	if fn, ok := m.fatalMethod(t, name); ok {
		return &methodFunc{t: t, fn: fn, replaced: true}
	}
	if fn, ok := refusedMethodOf(m, t, name); ok {
		return &methodFunc{t: t, fn: fn, replaced: true}
	}
	if fn, ok := execMethodOf(m, t, name); ok {
		return &methodFunc{t: t, fn: fn, replaced: true}
	}
	if fn, ok := rawConnMethodOf(m, t, name); ok {
		return &methodFunc{t: t, fn: fn, replaced: true}
	}
	if !m.realStd {
		if fn, ok := m.hostMethod(t, name); ok {
			return &methodFunc{t: t, fn: fn, replaced: true}
		}
		if fn, ok := restrictedMethods()[methodKey{t, name}]; ok {
			return &methodFunc{t: t, fn: fn, replaced: true}
		}
	}
	return lookupMethod(t, name)
}

// curMachine returns the machine of the interpreted code that runs on the
// current goroutine, or nil.
func curMachine() *Machine {
	if gs := lookupG(getg()); gs != nil && gs.cur != nil {
		return gs.cur.m
	}
	return nil
}

// AOTExit replaces os.Exit in the AOT copies (cmd/aotgen): it calls the
// os.Exit of the machine of the goroutine. On a goroutine that runs no
// interpreted code, it panics like the restricted os.Exit.
func AOTExit(code int) {
	if m := curMachine(); m != nil {
		m.osExitOf()(code)
		return
	}
	restrictedExit()(code)
}

// AOTFatal, AOTFatalf and AOTFatalln replace log.Fatal, log.Fatalf and log.Fatalln
// in the AOT copies (cmd/aotgen): they panic like the restricted ones,
// unless the machine of the goroutine has the standard library as in Go.
func AOTFatal(v ...any) {
	if m := curMachine(); m != nil && m.realStd {
		log.Output(2, fmt.Sprint(v...))
		m.osExitOf()(1)
		return
	}
	log.Panic(v...)
}

func AOTFatalf(format string, v ...any) {
	if m := curMachine(); m != nil && m.realStd {
		log.Output(2, fmt.Sprintf(format, v...))
		m.osExitOf()(1)
		return
	}
	log.Panicf(format, v...)
}

func AOTFatalln(v ...any) {
	if m := curMachine(); m != nil && m.realStd {
		log.Output(2, fmt.Sprintln(v...))
		m.osExitOf()(1)
		return
	}
	log.Panicln(v...)
}

// AOTArgs replaces the reads of os.Args in the AOT copies (cmd/aotgen): the
// arguments of the machine of the goroutine (its own copy when it is
// restricted).
func AOTArgs() []string {
	if m := curMachine(); m != nil && !m.realStd {
		return m.flags().args
	}
	return os.Args
}
