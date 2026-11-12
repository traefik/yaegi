package vm

// The standard input, output and error of a plugin (Options.Stdout and
// Options.Stderr).
//
// A host can give yaegi two writers that turn each write into a log line with
// the fields of the plugin. yaegi (fixStdlib in interp/use.go) then gives
// each interpreter its own fmt.Print, fmt.Printf and fmt.Println (they write
// to Stdout) and its own default logger of the log package (it writes to
// Stderr, and log.SetOutput changes only that logger). It changes os.Stdout
// and os.Stderr only when the writers are *os.File values.
//
// A Machine with SetStdio does the same with overrides of the native
// functions (as SetEnv does for the environment), and goes one step
// further for os.Stdin, os.Stdout and os.Stderr: the plugin gets its own
// variables, so an assignment in the plugin changes only its copy. When
// the writer is not an *os.File, the plugin's file is the write end of a
// pipe, and a goroutine copies what comes out of the pipe to the writer,
// one write per line when a read gives whole lines (a long line or a slow
// writer can split a line in two writes). These writes are asynchronous:
// they reach the writer a short time after the write of the plugin
// returns, and they can come after a later write of fmt.Println.
//
// A nil writer means the stream of the host, still through the plugin's
// own copies: fmt.Print* write to the host os.Stdout of the time of the
// write, the plugin's os.Stdout and os.Stderr start as the host files, and
// the plugin's default logger writes to the writer of the host default
// logger (log.Writer(), read at each write). So with nil writers too, the
// host variables os.Stdin, os.Stdout, os.Stderr and the settings of the
// host default logger (output, prefix, flags) never change.
//
// The machine also has its own default logger of log/slog (setSlog):
// slog.Default, slog.SetDefault, slog.SetLogLoggerLevel and the top-level
// functions (slog.Info...) are overrides, so the host slog.Default(), the
// level of its log bridge and its default logger of log never change. As in
// Go, slog.SetDefault with a handler of the program sends the machine's own
// default logger of log to that handler (SetOutput and SetFlags(0) on the
// machine's logger). Until then, slog.Default() is the default logger of
// the host log/slog (or of its AOT copy).
//
// The writers are called from several goroutines at the same time (the
// pipe goroutine, and fmt and the logger in each goroutine of the plugin):
// they must be safe for concurrent use.
//
// The print and println builtins write to Stderr, like the gc runtime
// (printOut), unless Machine.PrintOut is set.
//
// What this does not cover: code that runs natively (the standard library,
// AOT copies of libs) and writes to os.Stdout, os.Stderr or the default
// logger itself; the default logger of log/slog before slog.SetDefault (it
// writes to the host, not to Stderr); Close on the plugin's os.Stdout or
// os.Stderr when the writer is nil (the file is the host one). os.NewFile
// with the number of a host file descriptor panics in a restricted plugin
// (newfile.go).

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sync"
)

// SetStdio gives the machine its own standard output and standard error,
// and its own os.Stdin, os.Stdout, os.Stderr variables and default loggers
// of the log and log/slog packages. A nil writer means the stream of the host (see the
// top of this file); the globals of the host never change. The writers
// must be safe for concurrent use. It must be called before the program
// runs.
func (m *Machine) SetStdio(stdout, stderr io.Writer) {
	out := stdout
	if out == nil {
		out = hostWriter(func() io.Writer { return os.Stdout })
	}
	m.addOverride("fmt.Print", func(a ...any) (int, error) { return fmt.Fprint(out, a...) })
	m.addOverride("fmt.Printf", func(format string, a ...any) (int, error) { return fmt.Fprintf(out, format, a...) })
	m.addOverride("fmt.Println", func(a ...any) (int, error) { return fmt.Fprintln(out, a...) })
	// With fmt in ModeAOT, the calls go to the hooks of its AOT copy.
	if m.hookOverrides == nil {
		m.hookOverrides = map[string]NativeHook{}
	}
	m.hookOverrides["fmt.Print"] = fprintHook(out, "fmt.Fprint")
	m.hookOverrides["fmt.Printf"] = fprintHook(out, "fmt.Fprintf")
	m.hookOverrides["fmt.Println"] = fprintHook(out, "fmt.Fprintln")
	m.addVar("os.Stdin", nil, &os.Stdin)
	m.addVar("os.Stdout", stdout, &os.Stdout)
	m.addVar("os.Stderr", stderr, &os.Stderr)
	m.scanStdin()
	if stderr != nil {
		m.stderr = stderr
		m.setLogger(stderr)
	} else {
		m.setLogger(hostWriter(log.Writer))
	}
}

// fprintHook returns the AOT hook of fmt.Print, Printf or Println that
// calls the hook of fprint ("fmt.Fprintln") with w as first argument.
func fprintHook(w io.Writer, fprint string) NativeHook {
	return func(m *Machine, a []Value) Value {
		regMu.RLock()
		h := nativeHooks[fprint]
		regMu.RUnlock()
		if h == nil {
			unsupported("AOT hook %s not registered", fprint)
		}
		return h(m, append([]Value{Ret(m, w)}, a...))
	}
}

// hostWriter writes to the writer it returns at the time of each write:
// the host os.Stdout, or the writer of the host default logger.
type hostWriter func() io.Writer

func (w hostWriter) Write(p []byte) (int, error) { return w().Write(p) }

// addVar gives the machine its own *os.File variable for the native
// global key ("os.Stdout"), whose writes go to w (globalPtr reads
// m.varHooks). When w is nil, the variable starts as the host one (*host).
// The pipe of a writer that is not a file is made when the program first
// uses the variable.
func (m *Machine) addVar(key string, w io.Writer, host **os.File) {
	if m.varHooks == nil {
		m.varHooks = map[string]func() reflect.Value{}
	}
	m.varHooks[key] = sync.OnceValue(func() reflect.Value {
		var f *os.File
		if w == nil {
			f = *host // a copy: the plugin cannot change the host variable
		} else if fw, ok := w.(*os.File); ok {
			f = fw
		} else {
			var err error
			if f, err = pipeTo(w); err != nil {
				panic(fmt.Errorf("vm: %s of the plugin: %w", key, err))
			}
			return reflect.ValueOf(&f) // the address of the variable, as nativeVars
		}
		if m.restricted {
			// Its own duplicate of the host file: Close, Fd and
			// SyscallConn act on the duplicate (hostfd.go).
			f = ownStdFile(f, key != "os.Stdin")
		}
		return reflect.ValueOf(&f)
	})
}

// ownStdio gives a restricted machine without SetStdio (RunMain) its own
// os.Stdin, os.Stdout and os.Stderr: duplicates of the host files.
func (m *Machine) ownStdio() {
	for _, v := range []struct {
		key  string
		host **os.File
	}{{"os.Stdin", &os.Stdin}, {"os.Stdout", &os.Stdout}, {"os.Stderr", &os.Stderr}} {
		if _, ok := m.varHooks[v.key]; !ok {
			m.addVar(v.key, nil, v.host)
		}
	}
	m.scanStdin()
}

// scanStdin makes fmt.Scan, Scanln and Scanf read the machine's own
// os.Stdin at the time of the call, as they read os.Stdin in Go. The
// native functions read the host variable: a program that sets os.Stdin
// (to a file or a pipe) scanned the host input.
func (m *Machine) scanStdin() {
	stdin := func() io.Reader { return *m.varHooks["os.Stdin"]().Interface().(**os.File) }
	m.addOverride("fmt.Scan", func(a ...any) (int, error) { return fmt.Fscan(stdin(), a...) })
	m.addOverride("fmt.Scanln", func(a ...any) (int, error) { return fmt.Fscanln(stdin(), a...) })
	m.addOverride("fmt.Scanf", func(format string, a ...any) (int, error) { return fmt.Fscanf(stdin(), format, a...) })
	if m.hookOverrides == nil {
		m.hookOverrides = map[string]NativeHook{}
	}
	for name, fscan := range map[string]string{"fmt.Scan": "fmt.Fscan", "fmt.Scanln": "fmt.Fscanln", "fmt.Scanf": "fmt.Fscanf"} {
		m.hookOverrides[name] = func(m *Machine, a []Value) Value {
			regMu.RLock()
			h := nativeHooks[fscan]
			regMu.RUnlock()
			if h == nil {
				unsupported("AOT hook %s not registered", fscan)
			}
			return h(m, append([]Value{Ret(m, stdin())}, a...))
		}
	}
}

// devNull opens the null device in place of the host file f, which cannot
// be duplicated (ownStdFile).
func devNull(f *os.File, write bool) *os.File {
	flag := os.O_RDONLY
	if write {
		flag = os.O_WRONLY
	}
	n, err := os.OpenFile(os.DevNull, flag, 0)
	if err != nil {
		name := "stdio"
		if f != nil {
			name = f.Name()
		}
		panic(fmt.Errorf("vm: %s of the plugin: %w", name, err))
	}
	return n
}

// pipeTo returns the write end of a pipe whose data a goroutine copies to
// w, one write per line. The goroutine ends when the write end is closed:
// by the program, or by the cleanup of *os.File when the machine is gone.
func pipeTo(w io.Writer) (*os.File, error) {
	r, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer r.Close()
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Read(buf)
			for b := buf[:n]; len(b) > 0; {
				i := bytes.IndexByte(b, '\n') + 1
				if i == 0 {
					i = len(b) // the end of a line: written as is
				}
				w.Write(b[:i])
				b = b[i:]
			}
			if err != nil {
				return
			}
		}
	}()
	return pw, nil
}

// logFuncs are the functions of the log package that use the default
// logger, with the same name as the method of the logger. yaegi replaces
// the same list.
var logFuncs = []string{
	"Fatal", "Fatalf", "Fatalln", "Flags", "Output", "Panic", "Panicf", "Panicln",
	"Prefix", "Print", "Printf", "Println", "SetFlags", "SetOutput", "SetPrefix", "Writer",
}

// setLogger gives the machine its own default logger of the log package,
// which writes to w with the flags of the default logger of Go. It is made
// with the New of the machine's table, so its Fatal methods panic like
// log.Fatal in a plugin (stdsyms/restricted.go), or end the process with
// the standard library as in Go (realStd).
func (m *Machine) setLogger(w io.Writer) {
	syms := m.stdTableKey("log/log")
	newFn, ok := syms["New"]
	if !ok {
		return
	}
	l := newFn.Call([]reflect.Value{reflect.ValueOf(&w).Elem(), reflect.ValueOf(""), reflect.ValueOf(log.LstdFlags)})[0]
	if m.overrides == nil {
		m.overrides = map[string]reflect.Value{}
	}
	for _, name := range logFuncs {
		if meth := l.MethodByName(name); meth.IsValid() {
			m.overrides["log."+name] = meth
		}
	}
	m.overrides["log.Default"] = reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{l.Type()}, false),
		func([]reflect.Value) []reflect.Value { return []reflect.Value{l} })
	m.setSlog(l)
}

// slogState is the default logger of log/slog of a machine (setSlog).
type slogState struct {
	mu    sync.Mutex
	def   reflect.Value // the *slog.Logger of SetDefault; invalid before
	level reflect.Value // the slog.Level of SetLogLoggerLevel
}

// slogTopFuncs are the functions of log/slog that log with the default
// logger, with the same name as the method of *slog.Logger.
var slogTopFuncs = []string{
	"Debug", "DebugContext", "Info", "InfoContext", "Warn", "WarnContext",
	"Error", "ErrorContext", "Log", "LogAttrs",
}

// setSlog gives the machine its own default logger of log/slog, so that
// slog.SetDefault and slog.SetLogLoggerLevel in the program never change the
// host. In Go, SetDefault also makes the default logger of log write to the
// new handler (log.SetOutput and log.SetFlags(0)): here this changes logger,
// the machine's own default logger of log. Until the program calls
// SetDefault, slog.Default() and the top-level functions (slog.Info...) use
// the default logger of the host package (the native one, or the AOT copy
// of log/slog), as before. The functions are built on the symbols of the
// package the machine uses (native or AOT), with their own types.
func (m *Machine) setSlog(logger reflect.Value) {
	var syms map[string]reflect.Value
	if t := m.table("log/slog"); t != nil {
		syms = t.Funcs
	} else {
		syms = m.stdTableKey("log/slog/slog")
	}
	def, setDef, setLevel, newLog := syms["Default"], syms["SetDefault"], syms["SetLogLoggerLevel"], syms["NewLogLogger"]
	if !def.IsValid() || !setDef.IsValid() || !setLevel.IsValid() || !newLog.IsValid() {
		return
	}
	st := &slogState{level: reflect.Zero(setLevel.Type().In(0))} // LevelInfo is 0
	current := func() reflect.Value {
		st.mu.Lock()
		defer st.mu.Unlock()
		return st.def
	}
	m.overrides["log/slog.Default"] = reflect.MakeFunc(def.Type(), func(a []reflect.Value) []reflect.Value {
		if l := current(); l.IsValid() {
			return []reflect.Value{l}
		}
		return def.Call(a)
	})
	m.overrides["log/slog.SetDefault"] = reflect.MakeFunc(setDef.Type(), func(a []reflect.Value) []reflect.Value {
		l := a[0]
		h := l.MethodByName("Handler").Call(nil)[0] // panics on a nil logger, as in Go
		st.mu.Lock()
		st.def = l
		st.mu.Unlock()
		// As in Go: no bridge to the default handler, which writes to the
		// default logger of log (SetDefault(Default()) would loop).
		if !isSlogDefaultHandler(h) {
			var w io.Writer = &slogBridge{st: st, newLog: newLog, h: h}
			logger.MethodByName("SetOutput").Call([]reflect.Value{reflect.ValueOf(&w).Elem()})
			logger.MethodByName("SetFlags").Call([]reflect.Value{reflect.ValueOf(0)})
		}
		return nil
	})
	m.overrides["log/slog.SetLogLoggerLevel"] = reflect.MakeFunc(setLevel.Type(), func(a []reflect.Value) []reflect.Value {
		st.mu.Lock()
		defer st.mu.Unlock()
		old := st.level
		st.level = a[0]
		return []reflect.Value{old}
	})
	for _, name := range slogTopFuncs {
		fn := syms[name]
		if !fn.IsValid() {
			continue
		}
		m.overrides["log/slog."+name] = reflect.MakeFunc(fn.Type(), func(a []reflect.Value) []reflect.Value {
			if l := current(); l.IsValid() {
				return l.MethodByName(name).CallSlice(a)
			}
			return fn.CallSlice(a)
		})
	}
}

// isSlogDefaultHandler reports whether h (a slog.Handler) is the default
// handler of log/slog (native or AOT copy), which writes to the default
// logger of log.
func isSlogDefaultHandler(h reflect.Value) bool {
	if h.Kind() == reflect.Interface {
		h = h.Elem()
	}
	return h.IsValid() && h.Kind() == reflect.Pointer && h.Type().Elem().Name() == "defaultHandler"
}

// slogBridge is the writer that slog.SetDefault gives to the default logger
// of log of a machine: each write is a record of the handler h at the level
// of slog.SetLogLoggerLevel, as the bridge of log/slog (made with
// slog.NewLogLogger of the machine's package).
type slogBridge struct {
	st     *slogState
	newLog reflect.Value
	h      reflect.Value
}

func (b *slogBridge) Write(p []byte) (int, error) {
	b.st.mu.Lock()
	level := b.st.level
	b.st.mu.Unlock()
	l := b.newLog.Call([]reflect.Value{b.h, level})[0].Interface().(*log.Logger)
	return l.Writer().Write(p)
}
