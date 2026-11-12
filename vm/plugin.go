package vm

// Plugin loading API: load a package from its source, call its exported
// functions and the methods of the values they return, from native code.

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// Options configures plugin loading.
type Options struct {
	// Dir is the module root of the plugin (the plugin archive). When the
	// plugin package is in a nested module of Dir (a directory with its own
	// go.mod, whose module path is the import path of that directory), both
	// loaders use the go.mod and the vendor directory of that nested module,
	// as the go command does in it.
	Dir string
	// Root is the directory above which Load never looks for the go.mod of
	// the plugin. Dir must be Root or a directory below it (the two paths
	// are compared as given, then without their symbolic links). Without
	// Root, Load uses the go.mod of Dir or of its nearest parent that has
	// one, up to the root of the file system, as the go command does: a
	// plugin without a go.mod of its own then takes the go.mod of any
	// project above it, and fails with "not in module". With Root, Load
	// refuses such a plugin with "no go.mod in <Dir> or its parents up to
	// <Root>". A host sets it to the directory it owns: <goPath>/src for a
	// local plugin, the archive directory for a downloaded plugin. Both
	// loaders support it; with GoList, go list also runs without a go.work
	// file (GOWORK=off), as the default loader never reads one: a workspace
	// of the host's project above Root does not refuse the plugin.
	// LoadCached reads no go.mod.
	Root string
	// GoPath turns on the GOPATH mode of the default loader, as yaegi loads
	// Traefik local plugins (its GoPath option). It is off when empty. Go no
	// longer has this mode by default (GO111MODULE=on), so a host sets it
	// only for local plugins it already loaded this way, never for plugins it
	// downloads (they are modules).
	//
	// It only changes the load of a plugin that has no go.mod in Dir or its
	// parents up to Root (Root defaults to <GoPath>/src when only GoPath is
	// set). A plugin with a go.mod keeps module mode. Without a go.mod, Dir
	// must be in <GoPath>/src, and the package pkgPath is read from
	// <GoPath>/src/<pkgPath>. Each import that is not in the standard library
	// is read from the vendor directory of the importing package or of one
	// of its parents, then from <GoPath>/src/<import path> (gopath.go).
	// Nothing is read outside of <GoPath>/src: import paths with ".." or a
	// vendor element are refused, and so are directories that leave
	// <GoPath>/src through a symbolic link; a file that is a link is read
	// with the rule of the module files (followed only to a regular file in
	// <GoPath>/src, as in a Kubernetes ConfigMap volume). The
	// packages read this way have no module version: they are always
	// interpreted, never run as AOT copies, and use the Go version of the
	// host. The GoList loader (GoList or Src) refuses a plugin that needs
	// GOPATH mode: go list in GOPATH mode would read GOROOT and follow links
	// out of <GoPath>/src.
	GoPath string
	// Src lists standard library packages interpreted from their sources
	// instead of being called natively.
	Src []string
	// OpaqueAny passes interpreted values to native `any` parameters as opaque
	// handles (round-trip safe) instead of converting them to reflect-built
	// native copies.
	OpaqueAny bool
	// AOT runs the packages compiled ahead of time against the VM value model
	// (aot/..., registered with RegisterAOT) in ModeAOT.
	AOT bool
	// Modes sets the mode of individual import paths (see ModeFor).
	Modes map[string]Mode
	// GoList loads with go list (go/packages, needs the Go toolchain) instead
	// of reading the plugin module and its vendor directory directly and
	// type-checking against the embedded standard library types (stdtypes).
	// Implied by Src. Load never runs go list on its own: on a platform
	// without embedded types (stdtypes.Platforms), it returns an error
	// unless GoList is set. go list runs the Go toolchain on the plugin
	// sources before the load-time checks (restrict.go). It runs with a
	// minimal environment of its own (pluginGoListEnv): GOTOOLCHAIN=local,
	// GOPROXY=off, GOFLAGS=-mod=vendor (or -mod=readonly without a vendor
	// directory), whatever the host environment says, so the go.mod of the
	// plugin cannot make the host download a toolchain or modules.
	GoList bool
	// AllowStaleAOT lets AOT mode use AOT copies of the standard library
	// made with an older patch release of the Go release of the host (made
	// with go1.26.8, host built with go1.26.9). The copies then miss the
	// fixes of the newer patch releases, security fixes included. Without
	// it, Load refuses such copies: regenerate them (sh scripts/aotgen.sh)
	// with the Go version of the host. Copies of another Go release are
	// always refused. See aotversion.go.
	AllowStaleAOT bool
	// StaleAOTFallback loads the plugin in default mode, instead of
	// refusing it, when AOT mode is asked (AOT, or ModeAOT in Modes) and the
	// AOT copies cannot run in this host (CheckAOT(AllowStaleAOT) is not
	// nil: copies made with an older Go patch release, or another release).
	// No copy runs then, so no fix of the host is missed; the cost is the
	// behavior of AOT mode (the native standard library does not see the
	// methods of the plugin types: templates, slog, gob, xml, json/v2,
	// database/sql, net/rpc). Plugin.Notes then has one line that says so,
	// for a log. With AllowStaleAOT, copies of an older patch release run
	// and there is no fallback for them. See aotversion.go.
	StaleAOTFallback bool
	// The options below accept plugins that go build refuses, or load them
	// in a way that go build does not. They are all off by default: with
	// the zero Options, Load accepts a plugin only when go build would
	// build it, and loads the same files. They exist for plugins written
	// for yaegi, the interpreter that Traefik used before yaegi v2: yaegi does
	// not run the checks of the Go compiler, so a plugin that was only ever
	// run under yaegi may not be valid Go. A host that runs such plugins
	// turns them on (internal/pluginhost does); a host that runs plugins
	// built and tested with go build leaves them off.

	// IgnoreSoftErrors accepts a variable, an import or a label that is
	// declared and not used, and a := that declares no new variable. The Go
	// compiler refuses them ("declared and not used: x"). With the option,
	// the plugin runs as if the names were used. These are the "soft"
	// errors of go/types about unused names: the type information is
	// complete, so the code means the same. The other soft errors of
	// go/types (a type argument that does not satisfy its constraint, for
	// example) still refuse the plugin, with or without the option
	// (softerr.go). yaegi does not report unused names. Both loaders
	// support it.
	IgnoreSoftErrors bool
	// IgnoreUnusedExprs accepts a statement that is an expression whose
	// value is not used, like "c.S" or "len(s)" alone on a line. The Go
	// compiler refuses it ("c.S (variable of type string) is not used").
	// With the option, the expression is checked and run as usual, and its
	// value is dropped. The other errors of an expression statement still
	// refuse the plugin (softerr.go). yaegi runs such statements. Both
	// loaders support it.
	IgnoreUnusedExprs bool
	// IgnoreTestFiles leaves out the _test.go files when Load reads a
	// package. go build does not compile the test files either, but it
	// reads their package clause, and refuses a directory where a test file
	// is in another package than the package or <package>_test (a test file
	// in package main next to package plugin): "found packages plugin and
	// main". With the option, such a plugin loads. yaegi never reads the
	// test files, and a plugin host never runs the tests of a plugin. Both
	// loaders support it (testfiles.go). LoadCached never reads the test
	// files.
	IgnoreTestFiles bool
	// IgnoreDuplicateDecls accepts a package that declares a function, a
	// method, a type or a constant twice with the same text (a copy of a
	// file left next to the original). The Go compiler refuses it ("F
	// redeclared in this block"). With the option, the copies are removed
	// before the type check and the first declaration is kept; the copies
	// must be in files with the same imports and Go version. Two different
	// declarations of a name, and a variable declared twice, still refuse
	// the plugin: yaegi runs such a package too (it keeps the last function,
	// constant and variable, the first method, and runs the initializers of
	// both variables), but no valid Go program does the same (dupdecls.go).
	// Both loaders support it.
	IgnoreDuplicateDecls bool
	// YaegiTags reads the comment lines "// yaegi:tags t1 t2" of the files
	// of the plugin package and selects the files of the plugin and of its
	// dependencies with the build tags t1 and t2, as "go build -tags t1,t2"
	// does. Go ignores these comments, which are a directive of yaegi:
	// without the option, a file with the constraint "//go:build t1" is
	// left out, as go build leaves it out. A plugin written for yaegi may
	// need it (Coraza sets "tinygo" this way). Both loaders support it.
	YaegiTags bool
	// AllowNewerGo accepts a plugin whose go.mod (or a vendored module, in
	// vendor/modules.txt) asks for a Go release newer than the one yaegi v2
	// was built with ("go 1.27" when the host runs go1.26). The go command
	// refuses to build it with the release of the host ("go.mod requires go
	// >= 1.27") and downloads the newer toolchain when GOTOOLCHAIN allows
	// it; yaegi v2 has no other toolchain, so without the option Load refuses
	// the plugin. With the option, the plugin is type-checked as the
	// release of the host (the newer features of the language are errors)
	// and runs if it does not use them; a directive of its go.mod that the
	// host does not know is skipped, as the go command skips it in a newer
	// go.mod. yaegi does not read the go directive. The patch level is not
	// compared ("go 1.26.9" loads on go1.26.8), and the toolchain line is
	// not read (as with GOTOOLCHAIN=local). A go.mod without a go
	// directive is a go 1.16 module, as for the go command: this is not
	// "newer", the option does not change it. Default loader only: with
	// GoList, the go command decides (GOTOOLCHAIN).
	AllowNewerGo bool
	// ImportFromArchive lets a plugin that is a nested module (a directory
	// below Dir with its own go.mod, as plugins/traefik of darkweak/souin)
	// import a package of the module of Dir that its vendor directory does
	// not have: the package is read from its directory in the plugin
	// archive. The go command refuses this layout (a module gets the
	// packages of another module from its vendor directory or the module
	// cache, never from the directory next to it); yaegi reads the
	// directory. Default loader only: with GoList, the go command refuses
	// the import.
	ImportFromArchive bool
	// IgnoreInconsistentVendoring reads the vendor directory of the plugin
	// as it is, without comparing it with go.mod. go build refuses a module
	// whose go.mod and vendor/modules.txt do not agree ("inconsistent
	// vendoring": a module required in go.mod but not vendored, or vendored
	// but not required, or at another version, or replaced differently);
	// without the option, Load refuses it the same way, with the checks of
	// the go command (srcload.go, checkVendor). yaegi reads the
	// vendor directory and never reads go.mod, so a plugin tested only with
	// yaegi may have this. Default loader only: with GoList, the go command
	// makes the check (GOFLAGS=-mod=vendor).
	IgnoreInconsistentVendoring bool
	// UseUnsafe allows the imports of unsafe and syscall in a restricted
	// plugin (restrict.go). Of syscall, the plugin gets only a read-only
	// subset: process ids, user and group ids, page size, time of day, and
	// Getenv and Environ over its own environment copy (unsafesyscall.go).
	// The other syscall functions are refused at load time.
	UseUnsafe bool
	// Unrestricted lifts the load-time restrictions of plugins
	// (restrict.go) and gives the plugin the environment of the process
	// instead of its own copy (env.go). The plugin keeps the restricted
	// standard library: os.Exit, log.Fatal and the Fatal methods of the
	// loggers panic (a recover or GoPanic catches them), and os/exec and
	// syscall have no symbols (a call panics with "not in registry").
	Unrestricted bool
	// RealProcess, with Unrestricted and without Sandbox, also gives the
	// plugin the standard library as in Go: os.Exit, log.Fatal* and the
	// Fatal methods of the loggers end the host process at once (no
	// deferred call runs, nothing can recover), os.FindProcess works, and
	// os/exec and syscall have their symbols. Only for trusted code that
	// may end the host. A program that imports syscall is refused at load
	// time on a platform without a syscall table (stdtypes.Platforms).
	// Without Unrestricted, or with a Sandbox, RealProcess is ignored.
	RealProcess bool
	// Exec gives the plugin os/exec alone, without the rest of RealProcess
	// (os.Exit still panics, syscall keeps the rules of UseUnsafe):
	//   - ExecRefused (the default): a restricted plugin may not import
	//     os/exec, and an Unrestricted plugin gets no symbols of it.
	//   - ExecMock: the plugin loads and can use the API of os/exec
	//     (exec.Command, the fields of exec.Cmd...), but nothing runs:
	//     LookPath, os.StartProcess and the methods of *exec.Cmd that start a
	//     process (Start, Run, Output, CombinedOutput) return a *vm.ExecError
	//     (errors.Is(err, exec.ErrNotFound) is true). No process is started,
	//     nothing panics. Cmd.Environ gives the environment of the plugin
	//     (Env), not the one of the host, when Cmd.Env is not set. For a
	//     plugin that imports os/exec on a code path it does not use. A
	//     native template of such a plugin may not call a method named
	//     Start, Run, Output, CombinedOutput or Environ.
	//   - ExecReal: the real os/exec, and os.StartProcess. The plugin can
	//     run any program with the rights of the host process: the program
	//     can read and change everything the host can (files, network,
	//     other processes). Only for trusted code. A command gets the
	//     environment of the plugin (Env) when its Cmd.Env is not set.
	// RealProcess (with Unrestricted) gives the real os/exec whatever Exec
	// says. With a Sandbox, ExecReal acts as ExecMock: a Sandbox never runs
	// programs.
	Exec ExecMode
	// Env is the initial environment of a restricted plugin ("key=value"
	// entries); nil means a copy of os.Environ(), and an empty slice no
	// variable of the host.
	Env []string
	// Budget bounds every call from the host into the plugin (init, each
	// Plugin.Call and Object.Call, each ServeHTTP of a handler that
	// Object.Interface returns...) to that many
	// calls plus backward jumps; the call then panics with ErrBudget. The
	// callbacks that native code makes during the call and the goroutines
	// that the call starts (go statements, iter.Pull) share its budget, but
	// each call from the host has its own, also when it comes from inside
	// another call (the same plugin twice in a middleware chain). When the
	// call returns, the goroutines that still run, and the goroutines
	// started outside a call, are background goroutines: no total, but at
	// most Budget steps between two blocking points (a channel operation, a
	// select without default, time.Sleep), else they stop with ErrBudget
	// (GoPanic gets it). 0 means unlimited. See vm/budget.go and
	// docs/SANDBOX.md.
	Budget int64
	// BackgroundRate bounds the CPU of the background goroutines of the
	// plugin (see Budget): all of them together run at most BackgroundRate
	// steps per second, after a first burst of one second. A goroutine that
	// goes past it waits (a sleep) before its next steps: it is slowed down,
	// never stopped, so the program gives the same results as in Go, later.
	// Without it, a goroutine that blocks often (`for { work();
	// time.Sleep(time.Microsecond) }`) uses a CPU for ever, since Budget only
	// bounds the steps between two blocking points. The calls from the host,
	// and the goroutines they start while they run, are not slowed. Native
	// code is not counted. 0 (the default) means no bound, as in Go. A
	// Sandbox does not change it. See vm/budget.go.
	BackgroundRate int64
	// Cancel stops a call from the host when its context is done: ServeHTTP
	// (the context of the request, done when the client goes away),
	// Plugin.Call and Object.Call (their ctx). The call and its goroutines stop with
	// ErrCanceled at the next call, backward jump, channel wait or
	// time.Sleep; their deferred calls still run. The ServeHTTP of the host
	// then panics with http.ErrAbortHandler, which net/http turns into an
	// aborted response without a log. A blocking call of native code cannot
	// be stopped.
	//
	// Off by default: a call runs to its end, as in Go and yaegi, also when
	// its client is gone. Stopping it in the middle can leave the state of
	// the plugin half set (a sync.Once that is done, a counter between two
	// locks). The plugin still gets the context and can watch it, and Budget
	// still bounds the call. The init of LoadContext always stops at the
	// deadline of the load (see LoadTimeout).
	Cancel bool
	// RecoverHandler makes the ServeHTTP of a handler that Object.Interface
	// returns recover a panic: GoPanic gets it, and the response gets the
	// status 500 Internal Server Error (WriteHeader, as the recovery
	// middleware of Traefik does). http.ErrAbortHandler, and the end of a
	// request stopped by Cancel, end ServeHTTP quietly. Then nothing panics
	// out of ServeHTTP, also for a host that calls the handler on a goroutine
	// of its own without a recover, where a panic ends the process. A panic
	// of the next handler (host code that the plugin calls) is recovered too.
	//
	// Off by default: as in Go and yaegi, the host gets the panic (net/http
	// recovers it, logs it and closes the connection; ErrBudget and
	// ErrStackOverflow come to the host the same way).
	RecoverHandler bool
	// PanicOnFatal turns some fatal errors of the Go runtime into panics
	// of the plugin, when the VM can see them before they happen: Unlock of
	// an unlocked sync.Mutex, Unlock or RUnlock of an unlocked
	// sync.RWMutex (a lock of the plugin, not of the host), and concurrent
	// writes to a map of the plugin, or a read or a range of the map during
	// a write. The panic is a *FatalError: the plugin can recover it, and
	// GoPanic gets it in a goroutine of the plugin. In Go (and without the
	// option) these errors end the process, here the host. The same errors in
	// native code, out of memory and a deadlock still end it. Off by
	// default, as the behaviour is not the one of Go; a host that runs
	// plugins it does not trust turns it on. See vm/fatal.go.
	PanicOnFatal bool
	// MaxMemory is an approximate memory quota of the plugin, in bytes.
	// The VM counts its large allocations for the plugin (64 KiB or more:
	// make, the new array of an append, a large array, a string
	// concatenation) while they are alive, and the allocation that would go
	// past MaxMemory panics with ErrMemory instead (a recover can stop it;
	// GoPanic gets it in a goroutine). Smaller allocations, the allocations
	// of native code (strings.Repeat, io.ReadAll, the AOT copies) and the
	// memory of the VM itself are not counted: it stops a plugin that keeps
	// large buffers, not a plugin written to get around it. In Go a program
	// allocates until the process runs out of memory, so 0 (the default)
	// means no quota. See vm/memquota.go.
	MaxMemory int64
	// MaxStack bounds the stack of each goroutine of the plugin, in bytes:
	// the registers of its interpreted frames, the local variables that
	// stay in their frame (a local array) and the deferred calls that wait
	// (a defer in a loop). Past it, the goroutine stops with
	// ErrStackOverflow, as a Go program whose goroutine passes the 1 GB
	// stack limit of gc (there a fatal error). 0 means DefaultMaxStack
	// (1 GiB), <0 no bound (MaxDepth still bounds the frames). With
	// MaxMemory, a large register stack is also counted in the memory
	// quota. See vm/stacklimit.go.
	MaxStack int64
	// MaxGoroutines bounds the live goroutines of the plugin (go
	// statements and iter.Pull sequences). Past it, the go statement panics
	// with ErrTooManyGoroutines in the goroutine that runs it (a recover
	// can stop it). A func or a value of the plugin that native code runs
	// on a goroutine of its own (time.AfterFunc, context.AfterFunc, the
	// handlers of an http.Server that the plugin starts) counts as a
	// goroutine while it runs; past the cap, it does not run and GoPanic
	// gets ErrTooManyGoroutines (net/http gets it for a handler, and closes
	// the connection). Goroutines of native code that run no code of the
	// plugin are not counted. 0 means no cap. The count is for the life
	// of the machine: goroutines that never end (a janitor started in each
	// New) are only released by Plugin.Close (close.go).
	MaxGoroutines int
	// GoPanic receives the panics of the goroutines the plugin starts, and
	// of the callbacks that native code runs on goroutines of its own
	// (time.AfterFunc, context.AfterFunc, finalizers, a next handler that
	// native code calls on its own goroutine). nil prints them on
	// stderr. Either way the host process keeps running,
	// where Go (and yaegi) would end it. With RecoverHandler, it also gets
	// the panics of ServeHTTP.
	GoPanic func(r any)
	// PrintOut is where the print and println builtins of the plugin write.
	// nil means Stderr, else os.Stderr, like the gc runtime (yaegi writes
	// them to its Stdout).
	PrintOut io.Writer
	// Stdout and Stderr are the standard output and standard error of the
	// plugin, as the Stdout and Stderr options of yaegi (a host can give
	// writers that turn each line into a log line of the plugin). Stdout
	// gets fmt.Print, fmt.Printf, fmt.Println and the writes to os.Stdout;
	// Stderr gets the writes to os.Stderr, the default logger of the log
	// package (log.Print..., log.SetOutput changes the plugin's logger
	// only), the print and println builtins (unless PrintOut is set) and
	// the panics of the plugin goroutines (unless GoPanic is set). The
	// plugin sees its own os.Stdout and os.Stderr: the writer itself when
	// it is an *os.File, else a pipe to it, whose writes reach the writer
	// a short time later (stdio.go). The writers are called from several
	// goroutines at the same time: they must be safe for concurrent use.
	//
	// nil means the stream of the host, through the plugin's own copies:
	// fmt.Print* write to the host os.Stdout, the plugin's os.Stdout and
	// os.Stderr start as the host files, and the plugin's own default
	// logger writes to the writer of the host default logger. With or
	// without writers, the plugin cannot change the host variables
	// os.Stdin, os.Stdout and os.Stderr, the output, prefix and flags of
	// the host default logger of log, or the default logger and log level
	// of log/slog: an assignment, log.SetOutput, slog.SetDefault or
	// slog.SetLogLoggerLevel in the plugin changes its own copy only
	// (slog.SetDefault sends the plugin's log.Print... to the new handler,
	// as in Go). Until the plugin calls slog.SetDefault, slog.Default() and
	// slog.Info... use the default logger of the host log/slog (or of its
	// AOT copy), which neither Stdout nor Stderr gets.
	//
	// This is not isolation of the host streams. Not covered: the plugin
	// can call Close on its os.Stdout or os.Stderr, which with a nil writer
	// is the host *os.File itself, so it closes the host stdout or stderr.
	// (os.NewFile(1, ""), which wrote to the host fd 1 past these writers,
	// panics in a restricted plugin: newfile.go.)
	Stdout, Stderr io.Writer
	// Sandbox, when set, gives the plugin the environment it lists and
	// makes some os and net functions refuse the paths and hosts it does not
	// grant. It is a best effort against accidents, not a security boundary:
	// many other functions still reach the filesystem, the network and the
	// host's environment (sandbox.go, docs/SANDBOX.md).
	// It implies the restrictions (Unrestricted
	// is then ignored) and takes Env, Budget, MaxGoroutines and GoPanic
	// from the Sandbox rather than from the fields above.
	Sandbox *Sandbox
	// LoadTimeout bounds the whole Load (parse, type check, SSA build and
	// the package init). When it passes, Load returns a *LoadError instead
	// of blocking: a malicious plugin cannot block the configuration load
	// or reload of the host for ever (see loaddos.go). 0 means no timeout, unless the
	// context given to LoadContext carries a deadline. The type check has
	// no cancellation, so an abandoned load keeps a goroutine running in
	// the background until go/types returns; the timeout bounds the time
	// Load waits, not the resources of the abandoned load. An abandoned
	// load checks the context before the preflight and before the init, so
	// it does not start them after the deadline. A plugin (the same Dir,
	// GoPath and package) is loaded once at a time: a load waits, within its
	// own deadline, until an abandoned load of the same plugin has ended, so
	// loads that time out again and again do not pile up in the background
	// (a load with no deadline waits until that load ends; loads of other
	// plugins are not delayed). An init that is running
	// when the deadline passes is canceled like a call whose context is done
	// (see Budget): it stops at its next step, and the *LoadError also wraps
	// ErrCanceled. The default (0) is no deadline: a host that loads
	// untrusted plugins should set one, since a small
	// crafted plugin can take tens of seconds to type-check.
	LoadTimeout time.Duration
	// LoadLimits bounds the source that Load reads (total size, number of
	// files and packages) before type checking. nil uses DefaultLoadLimits.
	// It applies to the default loader only: with GoList or Src, the Go
	// toolchain (go list) reads the source and LoadLimits is not used.
	LoadLimits *LoadLimits
	// NoPreflight skips the load-time check (preflight.go). By default,
	// Load compiles every function the plugin can reach, in every branch,
	// and fails with a *PreflightError that lists the code the VM cannot
	// run (a native symbol missing from the registry, a function written in
	// assembly, syscall in a restricted plugin, an unsupported construct).
	// With NoPreflight, functions are compiled at their first call, and
	// these problems panic when the code runs, in the middle of a request.
	NoPreflight bool
	// StubAssembly loads a plugin that reaches functions without a Go body
	// (written in assembly, or linked to the runtime or to syscall with
	// //go:linkname) that the VM cannot run. By default the load-time check
	// refuses such a plugin, with the position of each function.
	//
	// With StubAssembly, each such function is a stub, and the rest of the
	// load-time check stays as it is:
	//   - a call of the stub panics with "unsupported", in the middle of the
	//     request (a recover catches it, as the other panics of a plugin);
	//   - the system calls that go through the C library on darwin and
	//     openbsd (golang.org/x/sys/unix, golang.org/x/sys/cpu and wazero
	//     link syscall_syscall6... to the syscall package, with the address
	//     of a C function held by assembly) return the error ENOSYS ("function
	//     not implemented") without calling the system: x/sys/cpu then sees
	//     no CPU feature, and unix.Mmap fails with an error;
	//   - in a restricted plugin with UseUnsafe, a function of syscall
	//     outside the read-only subset (syscall.Close, Write...) is a stub
	//     too: a call panics with a *RefusedError and calls nothing;
	//   - on the unix systems other than darwin, the system calls that
	//     golang.org/x/sys/unix makes with assembly (Syscall, Syscall6,
	//     RawSyscall, RawSyscall6) return ENOSYS, as on darwin;
	//   - when the plugin gets the host's golang.org/x/sys/unix (xsys.go)
	//     in a restricted plugin with UseUnsafe, a function outside its
	//     read-only subset returns ENOSYS (or panics with a *RefusedError
	//     when it has no error result), without calling the system.
	//
	// The VM never runs the assembly and never makes these system calls, so
	// this option does not give the plugin more access to the host than it
	// has without it: it needs no Unrestricted, and the syscall subset of
	// UseUnsafe does not change. The risk is for the plugin, not for the
	// memory of the host: the plugin runs differently from Go when it needs
	// one of these functions (a request fails where Go works, a probe of the
	// system sees an error). Use it for a plugin whose author knows that it
	// does not call them (zap and grpc reach unix.Mmap, wazero in interpreter
	// mode reaches the code of its compiler). Off by default; the catalog
	// host (internal/pluginhost) sets it, and a Traefik-like host should too:
	// zap and grpc then load and run as restricted plugins with useUnsafe.
	//
	// The helpers of the runtime that the VM gives with their Go meaning
	// (runtime.nanotime, runtime.getAuxv, runtime.memmove with the checks of
	// unsafe.Slice) work without this option (linkname.go).
	StubAssembly bool

	// hooks are set by the tests only (loadhooks.go).
	hooks *loadHooks

	// vmReflect: the program uses the VM reflect from the start (a Session,
	// whose inputs may import reflect later, eval.go).
	vmReflect bool
}

// restricted reports whether the load-time restrictions apply. A Sandbox
// always implies them, whatever Unrestricted says.
func (opts Options) restricted() bool { return opts.Sandbox != nil || !opts.Unrestricted }

// realStd reports whether the plugin gets the standard library as in Go
// (RealProcess).
func (opts Options) realStd() bool { return !opts.restricted() && opts.RealProcess }

// check applies the load-time checks to the packages run from source: the
// restrictions of plugins, or for a plugin with the real standard library,
// the imports that have no symbol table on this platform.
func (opts Options) check(src []sourcePkg) error {
	switch {
	case opts.restricted():
		if err := checkSource(src, opts.UseUnsafe, opts.Exec != ExecRefused); err != nil {
			return err
		}
	case opts.realStd():
		if err := checkRealStd(src, hasStdTable); err != nil {
			return err
		}
	}
	return checkSideEffectStd(src)
}

// preflight runs the load-time check of the plugin package root, unless
// opts.NoPreflight is set.
//
// The check compiles on a machine of its own (newM returns a new machine
// for the plugin), which is then dropped: the machine of the plugin still
// compiles each function at its first call, as without the check, and the
// code of the functions that never run does not stay in memory.
func (opts Options) preflight(p *Plugin, newM func() *Machine, own func(*types.Package) bool) error {
	if opts.NoPreflight {
		return nil
	}
	t := time.Now()
	err := newM().preflight(p.Pkg, own)
	p.CheckTime = time.Since(t)
	return err
}

// newMachine returns the machine of a plugin loaded with opts. own tells
// which interpreted packages are the plugin's own (the plugin and its
// non-standard dependencies): when one of them imports reflect, the plugin
// uses the VM reflect, as in RunMain (see ownReflect).
func (opts Options) newMachine(prog *ssa.Program, interp map[*types.Package]bool, own func(*types.Package) bool) *Machine {
	m := NewMachine(prog, interp)
	m.ownReflect = opts.vmReflect || ownReflect(interp, own)
	m.OpaqueAny = opts.OpaqueAny || opts.AOT || hasMode(opts.Modes, ModeAOT)
	m.AOT = opts.AOT
	m.Modes = opts.Modes
	m.PrintOut = opts.PrintOut
	if sb := opts.Sandbox; sb != nil {
		m.Budget = sb.Budget
		m.MaxGoroutines = sb.MaxGoroutines
		m.GoPanic = sb.GoPanic
	} else {
		m.Budget = opts.Budget
		m.MaxGoroutines = opts.MaxGoroutines
		m.GoPanic = opts.GoPanic
	}
	m.BackgroundRate = opts.BackgroundRate
	m.Cancel = opts.Cancel
	m.recoverHandler = opts.RecoverHandler
	m.panicOnFatal = opts.PanicOnFatal
	if opts.MaxMemory > 0 {
		m.mem = &memQuota{max: opts.MaxMemory, sweepAt: 64}
	}
	switch {
	case opts.MaxStack == 0:
		m.MaxStack = DefaultMaxStack
	case opts.MaxStack > 0:
		m.MaxStack = opts.MaxStack
	}
	if m.GoPanic == nil {
		stderr := opts.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		m.GoPanic = func(r any) { fmt.Fprintf(stderr, "vm: panic in a plugin goroutine: %v\n", r) }
	}
	m.restricted = opts.restricted()
	m.unsafeSyscall = opts.restricted() && opts.UseUnsafe
	m.stubAsm = opts.StubAssembly
	m.execMode = opts.Exec
	if opts.Sandbox != nil && m.execMode == ExecReal {
		m.execMode = ExecMock // a Sandbox never runs programs
	}
	switch {
	case opts.Sandbox != nil:
		opts.Sandbox.apply(m)
	case !opts.Unrestricted:
		env := opts.Env
		if env == nil {
			env = os.Environ()
		}
		m.SetEnv(env)
	default:
		m.realStd = opts.realStd()
	}
	// After realStd: the default logger comes from the table of the machine.
	m.SetStdio(opts.Stdout, opts.Stderr)
	m.setupExec()
	m.guardHostFiles()
	m.guardNativeGo()
	m.setupReflect()
	return m
}

// Plugin is a loaded plugin package.
type Plugin struct {
	M   *Machine
	Pkg *ssa.Package

	LoadTime  time.Duration // go/packages: go list + parse + type-check
	BuildTime time.Duration // SSA construction
	CheckTime time.Duration // load-time check (preflight.go)
	InitTime  time.Duration // package init in the VM

	// Notes are messages for the host about the load, which did not refuse
	// the plugin. A note is one line, for a log (WARN). Today the only note
	// says that the build constraints of a source package select other
	// files than yaegi did: yaegi reads only "// +build" lines, so a file
	// with only a "//go:build" line was always kept. yaegi v2 selects the files
	// as go build does; the note helps to find a plugin that now runs other
	// code than under yaegi. It is nil when there is nothing to say.
	Notes []string
}

// Load loads pkgPath from source (plus opts.Src packages) and runs its init.
// It bounds the load with opts.LoadTimeout (see LoadContext and loaddos.go).
func Load(opts Options, pkgPath string) (*Plugin, error) {
	return LoadContext(context.Background(), opts, pkgPath)
}

// LoadContext is Load with a context. The context (and opts.LoadTimeout)
// bound the whole load: when the context is cancelled or its deadline, or
// opts.LoadTimeout, passes, LoadContext returns a *LoadError at once.
//
// When this happens before the package init starts, the load does not run
// the init (see preflightAndInit). When it happens while the init runs, the
// init is also cancelled, like a call with a done context (see
// Options.Budget): it stops at its next budget check, and the *LoadError
// also wraps ErrCanceled, so both errors.As(err, new(*LoadError)) and
// errors.Is(err, ErrCanceled) are true.
//
// The load runs in its own goroutine so that LoadContext can return on the
// deadline even while go/types, which has no cancellation, is still checking
// a pathological package; that goroutine keeps running in the background
// until go/types returns (see loaddos.go).
func LoadContext(ctx context.Context, opts Options, pkgPath string) (*Plugin, error) {
	if opts.LoadTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.LoadTimeout)
		defer cancel()
	}
	release, err := acquireLoadSlot(ctx, &opts, pkgPath)
	if err != nil {
		return nil, err
	}
	if ctx.Done() == nil {
		// No deadline and no cancellation: run in this goroutine, as before.
		defer release()
		p, err := loadImpl(ctx, &opts, pkgPath)
		return p, opts.hidePaths(err)
	}
	initStarted := new(atomic.Bool)
	ctx = context.WithValue(ctx, initStartedKey{}, initStarted)
	type result struct {
		p   *Plugin
		err error
	}
	ch := make(chan result, 1)
	go func() {
		// The slot is released when the load ends, also when LoadContext
		// gave up on it: the next load of the plugin waits for it.
		defer release()
		p, err := loadImpl(ctx, &opts, pkgPath)
		ch <- result{p, err}
	}()
	select {
	case <-ctx.Done():
		if initStarted.Load() {
			return nil, initCanceledErr(ctx)
		}
		return nil, loadErrorf("vm load: %v (parse or type checking has no cancellation, so the abandoned load keeps running in the background)", ctx.Err())
	case r := <-ch:
		if r.err != nil && ctx.Err() != nil && errors.Is(r.err, ErrCanceled) {
			// The init stopped on the done context before the select saw it.
			return nil, initCanceledErr(ctx)
		}
		return r.p, opts.hidePaths(r.err)
	}
}

// loadKey names a plugin for loadSlots: the same package from the same
// directory.
type loadKey struct{ dir, goPath, pkgPath string }

// loadSlots holds one slot (a channel with room for one value) per plugin
// (loadKey). go/types and the SSA builder cannot be canceled, so a load that
// LoadContext gave up on keeps running in the background with all its memory
// until they return. Without the slot, each new load of the same plugin (a
// host that loads it again at each configuration reload, or that retries
// after the error) started one more: 6 loads of a crafted 8 MiB package, one
// per second with a LoadTimeout of 1 s, used 3.7 GiB together. With the slot,
// a load of a plugin waits until the previous load of the same plugin has
// ended, within its own deadline. The entries are never removed: one small
// channel per plugin loaded by the process.
var loadSlots sync.Map // loadKey -> chan struct{}

// acquireLoadSlot takes the slot of the plugin pkgPath of opts.Dir, waiting
// for a previous load of the same plugin to end, until ctx is done. release
// gives the slot back.
func acquireLoadSlot(ctx context.Context, opts *Options, pkgPath string) (release func(), err error) {
	key := loadKey{dir: opts.Dir, goPath: opts.GoPath, pkgPath: pkgPath}
	if abs, err := filepath.Abs(opts.Dir); err == nil {
		key.dir = abs
	}
	v, _ := loadSlots.LoadOrStore(key, make(chan struct{}, 1))
	slot := v.(chan struct{})
	release = func() { <-slot }
	select {
	case slot <- struct{}{}: // free: never refused for a done context
		return release, nil
	default:
	}
	select {
	case slot <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, loadErrorf("vm load: %v: a previous load of %s has not ended yet (go/types and the SSA builder cannot be canceled, so a load abandoned after its deadline runs until they return; a plugin is loaded once at a time)",
			ctx.Err(), pkgPath)
	}
}

// initStartedKey is the context key of the *atomic.Bool that
// preflightAndInit sets just before it runs the package init.
type initStartedKey struct{}

// initCanceledErr is the error of LoadContext when its context is done while
// the package init runs: a *LoadError that also wraps ErrCanceled.
func initCanceledErr(ctx context.Context) error {
	return &LoadError{Msg: fmt.Sprintf("vm load: %v: package init canceled", ctx.Err()), Err: ErrCanceled}
}

// loadImpl, loadPlugin, preflightAndInit and loadCached take the Options by
// pointer, not by value. Options is 240 bytes, so a by-value copy lives in
// the arguments area of the frame, and the closures that use it (newM)
// take its address: it is then a "stack object" of the GC. The runtime bug
// go.dev/issue/82394 (seen on linux/amd64, Go 1.22 to 1.27) makes such an
// object reachable again while a frame runs its deferred calls during a
// panic: the _panic record keeps the frame pointer, which on amd64 is the
// address of the arguments, and the stack scan follows it. A panic of the
// package init, recovered here, scanned the dead Options of this frame,
// whose Dir string was already freed: "found pointer to free object"
// (TestLoadPanicGC). A pointer argument lives in a register, so the frame
// has no such object.
func loadImpl(ctx context.Context, opts *Options, pkgPath string) (p *Plugin, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicErr("vm load", r)
			opts.hooks.loadStage("recovered")
		}
	}()
	var aotNote string
	opts.AOT, opts.Modes, aotNote, err = aotGate(opts.hooks.hostGoVersion(), opts.AOT, opts.Modes, opts.AllowStaleAOT, opts.StaleAOTFallback)
	if err != nil {
		return nil, err
	}
	if aotNote != "" {
		defer func() {
			if p != nil {
				p.Notes = append(p.Notes, aotNote)
			}
		}()
	}
	if len(opts.Src) == 0 && !opts.GoList {
		// Fail closed: go list only runs when the host asks for it.
		if !opts.hooks.haveStdTypes() {
			return nil, fmt.Errorf("vm: yaegi v2 has no standard library types for %s/%s (only for %s): set Options.GoList to load plugins with the Go toolchain (go list)",
				runtime.GOOS, runtime.GOARCH, strings.Join(stdtypes.Platforms, ", "))
		}
		return loadPlugin(ctx, opts, pkgPath)
	}
	t0 := time.Now()
	limit := opts.modLimit()
	var env []string
	if limit != "" {
		// go list finds the same go.mod as findModule (the nearest one),
		// but it does not stop at Options.Root: refuse the load here when
		// there is no go.mod up to Root.
		if _, _, _, err := findModule(opts.Dir, limit); err != nil {
			if errors.Is(err, errNoGoMod) && opts.GoPath != "" {
				// go list with GO111MODULE=off would also resolve
				// imports in GOROOT/src and follow symbolic links out
				// of <GoPath>/src: GOPATH mode is for the default
				// loader only.
				return nil, fmt.Errorf("%w: GOPATH mode (Options.GoPath) needs the default loader, not Options.GoList or Options.Src", err)
			}
			return nil, err
		}
		// go list also reads the go.work of the nearest parent of the
		// module, even above Root: the workspace of the host's project,
		// which does not list the plugin module, refuses it ("main module
		// ... does not contain package ..."). Without a workspace, go list
		// reads the plugin module alone, as the default loader does (it
		// never reads go.work).
		env = append(env, "GOWORK=off")
	}
	// go list runs in the module of the plugin package: a nested module
	// (its own go.mod and vendor/) is not part of the module of opts.Dir.
	listDir := pluginModuleDir(opts.Dir, limit, pkgPath)
	env = append(pluginGoListEnv(os.Environ(), listDir), env...)
	var testFiles []string
	if opts.IgnoreTestFiles {
		if testFiles, err = moduleTestFiles(listDir, limit); err != nil {
			return nil, err
		}
	}
	// With Options.YaegiTags, go list selects the files with the build tags
	// of the "// yaegi:tags" lines of the plugin package, as the default
	// loader does. go list is the only way to find the directory of the
	// package here (a load of its name and files only).
	var buildTags []string
	if opts.YaegiTags {
		if buildTags, err = goListYaegiTags(ctx, listDir, env, pkgPath, opts.hooks); err != nil {
			return nil, err
		}
	}
	// dupSrcs holds the files whose repeated declarations are blanked
	// (Options.IgnoreDuplicateDecls, dupdecls.go): go list reads them
	// through an overlay, and go/packages parses them through ParseFile.
	dupSrcs := map[string][]byte{}
	loadOnce := func(mode packages.LoadMode) ([]*packages.Package, error) {
		cfg := &packages.Config{Mode: mode | packages.NeedModule, Dir: listDir, Context: ctx, Env: env}
		if len(testFiles) > 0 || len(dupSrcs) > 0 {
			flag, cleanup, err := writeOverlay(testFiles, dupSrcs)
			if err != nil {
				return nil, err
			}
			defer cleanup()
			cfg.BuildFlags = []string{flag}
		}
		if len(buildTags) > 0 {
			cfg.BuildFlags = append(cfg.BuildFlags, "-tags="+strings.Join(buildTags, ","))
		}
		cfg.ParseFile = func(fset *token.FileSet, name string, src []byte) (*ast.File, error) {
			if s, ok := dupSrcs[name]; ok {
				src = s
			}
			return parseFile(fset, name, src)
		}
		pkgs, err := opts.hooks.loadPackages(cfg, append([]string{pkgPath}, opts.Src...)...)
		if err != nil {
			return nil, err
		}
		if pol := opts.typeErrPolicy(); pol.any() {
			dropSoftErrors(pkgs, pol)
		}
		return pkgs, nil
	}
	loadPkgs := func(mode packages.LoadMode) ([]*packages.Package, error) {
		pkgs, err := loadOnce(mode)
		// With IgnoreDuplicateDecls, a package with errors is loaded again
		// with its repeated declarations blanked. Each new load blanks more
		// files (or stops): a few rounds are enough for the dependencies
		// that the first load did not type-check from source.
		for round := 0; err == nil && opts.IgnoreDuplicateDecls && round < 3 && hasPkgErrors(pkgs); round++ {
			more, derr := dupSources(pkgs, dupSrcs)
			if derr != nil || len(more) == 0 {
				break
			}
			for name, src := range more {
				dupSrcs[name] = src
			}
			pkgs, err = loadOnce(mode)
		}
		if err != nil {
			return nil, err
		}
		dropTypeErrorsAfterSyntax(pkgs)
		joinSubErrors(pkgs)
		if packages.PrintErrors(pkgs) > 0 {
			return nil, errors.New("package errors")
		}
		return pkgs, nil
	}
	pkgs, err := loadPkgs(packages.LoadSyntax)
	if err != nil {
		return nil, err
	}
	// As in RunMain: packages outside the standard library (the plugin's own
	// subpackages, its vendored modules) have no native registry entry, so
	// they are interpreted (or run in AOT mode), which requires their syntax.
	nonStd := nonStdDeps(pkgs)
	// The tree of a package of opts.Src that has its own internal package
	// is interpreted with it (srcFamily), which needs its syntax too.
	family := srcFamily(opts.Src, loadedPaths(pkgs))
	allSyntax := len(nonStd) > 0 || usesInterpretedStd(pkgs) || len(family) > 0
	if allSyntax {
		if pkgs, err = loadPkgs(packages.LoadAllSyntax); err != nil {
			return nil, err
		}
		nonStd = nonStdDeps(pkgs)
	}
	// A package with assembly takes its pure Go files, as with the default
	// loader (goasm.go).
	var yaegiExtra []string
	stdSrc := func(path string) bool { return InterpretedStd[path] || slices.Contains(opts.Src, path) || family[path] }
	if needsAsmTags(pkgs, func(path string) bool { return path == pkgPath || nonStd[path] }, stdSrc, buildTags) {
		buildTags = append(buildTags, asmTags...)
		yaegiExtra = asmTags
		mode := packages.LoadSyntax
		if allSyntax {
			mode = packages.LoadAllSyntax
		}
		if pkgs, err = loadPkgs(mode); err != nil {
			return nil, err
		}
		nonStd = nonStdDeps(pkgs)
	}
	var fallback, natFallback map[string]string
	info := goListInfo(pkgs)
	opts.Modes, fallback = aotFallbacks(opts.Modes, opts.AOT, mapKeys(nonStd), info)
	// The plugin and its non-standard dependencies, not the standard
	// library packages of opts.Src (they import unsafe).
	src := sourcePkgsOf(pkgs, func(path string) bool { return path == pkgPath || nonStd[path] })
	opts.Modes, natFallback = xsysModes(opts.Modes, opts.xsysWanted(), src, info)
	src = dropNative(src, opts.Modes, opts.AOT)
	opts.Modes = stdAOTModes(opts.Modes, opts.AOT, src, opts.hooks.hostGoVersion(), opts.AllowStaleAOT, stdSrc, !opts.realStd()) // aot.go
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	t1 := time.Now()
	var prog *ssa.Program
	var spkgs []*ssa.Package
	if allSyntax {
		prog, spkgs = ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	} else {
		prog, spkgs = ssautil.Packages(pkgs, ssa.InstantiateGenerics)
	}
	interp := map[*types.Package]bool{}
	for _, sp := range spkgs {
		if sp == nil {
			return nil, errors.New("ill-typed package")
		}
		sp.Build()
		interp[sp.Pkg] = true
	}
	if allSyntax {
		prog.Build()
		for _, sp := range prog.AllPackages() {
			path := sp.Pkg.Path()
			if nonStd[path] && ModeFor(opts.Modes, opts.AOT, path) == ModeInterp || InterpretedStd[path] || family[path] {
				interp[sp.Pkg] = true
			}
		}
	}
	if err := opts.check(src); err != nil {
		return nil, err
	}
	embeds, err := embedsOf(src, opts.loadLimits())
	if err != nil {
		return nil, &LoadError{Msg: err.Error()}
	}
	links := linknamesOf(src)
	if err := checkAOTSymbols(opts.Modes, opts.AOT, src); err != nil {
		return nil, err
	}
	t2 := time.Now()
	own := func(p *types.Package) bool { return p.Path() == pkgPath || nonStd[p.Path()] }
	fieldTrack := goListFieldTrack(listDir, pkgs)
	dropSyntaxAll(pkgs) // after the checks (shrink.go)
	newM := func() *Machine {
		m := opts.newMachine(prog, interp, own)
		m.AOTFallback = fallback
		m.NativeFallback = natFallback
		m.FieldTrack = fieldTrack
		m.embeds = embeds
		m.linknames = links
		return m
	}
	p = &Plugin{LoadTime: t1.Sub(t0), BuildTime: t2.Sub(t1)}
	p.Notes = goListYaegiNotes(pkgs, pkgPath, yaegiExtra, func(path string) bool { return path == pkgPath || nonStd[path] })
	for _, sp := range spkgs {
		if sp.Pkg.Path() == pkgPath {
			p.Pkg = sp
		}
	}
	if p.Pkg == nil {
		return nil, fmt.Errorf("package %s not found (an import path is expected)", pkgPath)
	}
	if err := preflightAndInit(ctx, opts, p, newM, own); err != nil {
		return nil, err
	}
	p.InitTime = time.Since(t2) - p.CheckTime
	return p, nil
}

// preflightAndInit runs the preflight of p, then creates its machine and
// runs the package init. It checks ctx before each step: when the deadline
// passes during the SSA build (one ssa.Package.Build cannot be interrupted),
// LoadContext has already returned an error, and the abandoned load must not
// run the code of the plugin that the host refused.
func preflightAndInit(ctx context.Context, opts *Options, p *Plugin, newM func() *Machine, own func(*types.Package) bool) error {
	opts.hooks.loadStage("built")
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if err := opts.preflight(p, newM, own); err != nil {
		return err
	}
	opts.hooks.loadStage("checked")
	if err := ctxErr(ctx); err != nil {
		return err
	}
	m := newM()
	p.M = m
	if inits := m.initOrder(p.Pkg); len(inits) > 0 {
		if started, ok := ctx.Value(initStartedKey{}).(*atomic.Bool); ok {
			started.Store(true)
		}
		defer opts.hooks.loadStage("init")
		m.runInits(ctx, inits)
	}
	return nil
}

// loadPlugin is Load without the Go toolchain (see srcload.go).
func loadPlugin(ctx context.Context, opts *Options, pkgPath string) (*Plugin, error) {
	r, err := loadSource(ctx, opts.Dir, opts.modLimit(), opts.GoPath, pkgPath, opts.loadLimits(), opts.loadPolicy())
	if err != nil {
		return nil, err
	}
	var natFallback map[string]string
	opts.Modes, natFallback = xsysModes(opts.Modes, opts.xsysWanted(), r.src, r.info)
	src := dropNative(r.src, opts.Modes, opts.AOT)
	opts.Modes = stdAOTModes(opts.Modes, opts.AOT, src, opts.hooks.hostGoVersion(), opts.AllowStaleAOT, func(p string) bool { return InterpretedStd[p] }, !opts.realStd()) // aot.go
	if err := opts.check(src); err != nil {
		return nil, err
	}
	embeds, err := embedsOf(src, opts.loadLimits())
	if err != nil {
		return nil, &LoadError{Msg: err.Error()}
	}
	links := linknamesOf(src)
	var paths []string
	for _, sp := range r.pkgs {
		paths = append(paths, sp.Pkg.Path())
	}
	var fallback map[string]string
	opts.Modes, fallback = aotFallbacks(opts.Modes, opts.AOT, paths, r.info)
	if err := checkAOTSymbols(opts.Modes, opts.AOT, src); err != nil {
		return nil, err
	}
	dropSyntax(r.src) // after the checks (shrink.go)
	t0 := time.Now()
	interp := map[*types.Package]bool{r.root.Pkg: true}
	for _, sp := range r.stdSrc {
		if InterpretedStd[sp.Pkg.Path()] {
			interp[sp.Pkg] = true
		}
	}
	for _, sp := range r.pkgs {
		// Packages of the module and vendored modules have no native
		// registry entry: interpreted, or AOT copies (ModeFor).
		if ModeFor(opts.Modes, opts.AOT, sp.Pkg.Path()) == ModeInterp {
			interp[sp.Pkg] = true
		}
	}
	own := map[*types.Package]bool{}
	for _, sp := range r.pkgs {
		own[sp.Pkg] = true // the module and its vendored modules
	}
	isOwn := func(p *types.Package) bool { return own[p] }
	newM := func() *Machine {
		m := opts.newMachine(r.prog, interp, isOwn)
		m.AOTFallback = fallback
		m.NativeFallback = natFallback
		m.FieldTrack = buildFieldTrack() // the build tags of the loader
		m.embeds = embeds
		m.linknames = links
		return m
	}
	p := &Plugin{Pkg: r.root, LoadTime: r.loadTime, BuildTime: r.buildTime, Notes: r.notes}
	if err := preflightAndInit(ctx, opts, p, newM, isOwn); err != nil {
		return nil, err
	}
	p.InitTime = time.Since(t0) - p.CheckTime
	return p, nil
}

// Object is an interpreted value given to the host: a result of Plugin.Call
// or Object.Call. The host passes it back as an argument of these calls,
// converts it to a native value (Interface), fills it (Decode, SetFields),
// or calls its methods (Call).
type Object struct {
	p *Plugin
	v Value
	t types.Type
}

// Type returns the static type of o: the type of the result it comes from.
func (o *Object) Type() types.Type { return o.t }

// Call calls the exported function name of the plugin package and returns
// its results.
//
// An argument is either an *Object of this plugin, passed as is (its type
// must be the type of the parameter), or a native value, converted to the
// type of the parameter (nil gives the zero value). A native http.Handler
// given for an interface parameter is wrapped: when native code that the
// plugin started calls it on a goroutine of its own, its panics go to
// Options.GoPanic instead of ending the host.
//
// The call is a call from the host: it has its own budget (Options.Budget),
// and it stops when ctx is done with Options.Cancel. A panic of the function
// (ErrBudget too) is returned as an error, and a runtime.Goexit as
// ErrGoexit.
func (p *Plugin) Call(ctx context.Context, name string, args ...any) (res []*Object, err error) {
	defer recoverErr(&err)
	f := p.Pkg.Func(name)
	if f == nil || !token.IsExported(name) {
		return nil, fmt.Errorf("package %s has no exported function %s", p.Pkg.Pkg.Path(), name)
	}
	return p.call(ctx, f, nil, args)
}

// Call calls the exported method name of o, as Plugin.Call does for a
// function. The type of o must not be an interface type.
func (o *Object) Call(ctx context.Context, name string, args ...any) (res []*Object, err error) {
	defer recoverErr(&err)
	if types.IsInterface(o.t) {
		return nil, fmt.Errorf("method %s of the interface %s: a value of a concrete type is expected", name, o.t)
	}
	m := o.p.M
	sel := m.prog.MethodSets.MethodSet(o.t).Lookup(nil, name)
	if sel == nil || !token.IsExported(name) {
		return nil, fmt.Errorf("%s has no exported method %s", o.t, name)
	}
	recv := o.v
	return o.p.call(ctx, m.prog.MethodValue(sel), &recv, args)
}

// call calls fn, with the receiver recv when fn is a method.
func (p *Plugin) call(ctx context.Context, fn *ssa.Function, recv *Value, args []any) ([]*Object, error) {
	m := p.M
	if m.refused() {
		return nil, ErrClosed // close.go
	}
	sig := fn.Signature
	if sig.Variadic() {
		return nil, fmt.Errorf("%s: variadic functions are not supported", fn.Name())
	}
	if sig.Params().Len() != len(args) {
		return nil, fmt.Errorf("%s: %d arguments given, %d expected", fn.Name(), len(args), sig.Params().Len())
	}
	in := make([]Value, 0, len(args)+1)
	if recv != nil {
		in = append(in, *recv)
	}
	for i, a := range args {
		v, err := p.arg(a, sig.Params().At(i).Type())
		if err != nil {
			return nil, fmt.Errorf("%s: argument %d: %w", fn.Name(), i, err)
		}
		in = append(in, v)
	}
	out := m.callTop(m.cancelCtx(ctx), m.funcOf(fn), in, nil)
	n := sig.Results().Len()
	if n == 0 {
		return nil, nil
	}
	vals := []Value{out}
	if n > 1 {
		vals = out.R.([]Value)
	}
	res := make([]*Object, n)
	for i, v := range vals {
		res[i] = &Object{p: p, v: v, t: sig.Results().At(i).Type()}
	}
	return res, nil
}

// arg converts the argument a for a parameter of type pt.
func (p *Plugin) arg(a any, pt types.Type) (Value, error) {
	m := p.M
	if o, ok := a.(*Object); ok {
		if o.p != p {
			return Value{}, errors.New("the object comes from another plugin")
		}
		if !types.Identical(o.t, pt) {
			return Value{}, fmt.Errorf("the object has the type %s, %s is expected", o.t, pt)
		}
		return o.v, nil
	}
	rt := m.reflectType(pt)
	if a == nil {
		return m.fromNative(reflect.Zero(rt), pt), nil
	}
	if h, ok := a.(http.Handler); ok && rt.Kind() == reflect.Interface {
		a = &nativeNext{m, h}
	}
	rv := reflect.ValueOf(a)
	if rt.Kind() == reflect.Interface && rv.Type().Implements(rt) {
		// The parameter keeps its interface type, as in a Go call.
		iv := reflect.New(rt).Elem()
		iv.Set(rv)
		rv = iv
	}
	return m.fromNative(rv, pt), nil
}

// Interface returns the native value of o. An error value gives a native
// error, a nil one gives nil. A value that is a net/http.Handler (of type
// http.Handler, or of an interface type of the plugin with the ServeHTTP
// method) gives a handler whose each ServeHTTP is a call from the host: it has its own
// budget, and it stops when the context of the request is done with
// Options.Cancel.
func (o *Object) Interface() (v any, err error) {
	defer recoverErr(&err)
	m := o.p.M
	rv := m.toNative(o.v, o.t)
	if !rv.IsValid() {
		return nil, nil
	}
	v = rv.Interface()
	// Any value that is an http.Handler, not only one of static type
	// http.Handler: an interface of the plugin with a ServeHTTP method gives
	// one too, and its ServeHTTP must also be a call from the host.
	if h, ok := v.(http.Handler); ok && h != nil {
		if _, ok := h.(topHandler); !ok {
			return topHandler{m, h}, nil
		}
	}
	return v, nil
}

// Decode lets a native decoder fill o, which must be a pointer: decode
// receives a native copy of the pointer, which is copied back into the
// interpreted value afterwards (a decoder such as mapstructure or
// encoding/json cannot write into interpreted memory). When the pointed type
// is recursive (a struct that holds a slice, map or pointer of itself), the
// native copy has a finite depth: decode is then called again on a deeper
// copy while the decoded data goes past that depth (see recursive.go), so it
// must decode the same data at each call.
func (o *Object) Decode(decode func(ptr any) error) (err error) {
	defer recoverErr(&err)
	m := o.p.M
	pt, ok := o.t.Underlying().(*types.Pointer)
	cell, ok2 := o.v.R.(*Value)
	if !ok || !ok2 {
		return fmt.Errorf("the object is not an interpreted pointer: %s", o.t)
	}
	if m.hasCut(pt.Elem()) {
		return m.decodeConfigShaped(o.v, o.t, decode)
	}
	rv := m.toNative(o.v, o.t)
	if err := decode(rv.Interface()); err != nil {
		return err
	}
	*cell = m.fromNativeDeep(rv.Elem(), pt.Elem())
	return nil
}

// SetFields sets fields (by Go field name) of the struct that o, a pointer
// to a struct, points to.
func (o *Object) SetFields(fields map[string]any) (err error) {
	defer recoverErr(&err)
	pt, ok := o.t.Underlying().(*types.Pointer)
	cell, ok2 := o.v.R.(*Value)
	if !ok || !ok2 {
		return fmt.Errorf("the object is not an interpreted pointer: %s", o.t)
	}
	st, ok := pt.Elem().Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("the object does not point to a struct: %s", o.t)
	}
	fs := cell.R.([]Value)
	for name, v := range fields {
		found := false
		for i := 0; i < st.NumFields(); i++ {
			if st.Field(i).Name() == name {
				fs[i] = o.p.M.fromNativeDeep(reflect.ValueOf(v), st.Field(i).Type())
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s has no field %s", pt.Elem(), name)
		}
	}
	return nil
}

func recoverErr(err *error) {
	if r := recover(); r != nil {
		*err = panicErr("vm", r)
	}
}

// panicErr returns the error for the panic value r of plugin code.
func panicErr(prefix string, r any) error {
	if r == errGoexit {
		return ErrGoexit
	}
	if e, ok := r.(error); ok {
		// Keep the error in the chain: errors.Is(err, ErrBudget) works, and
		// a *LoadError from a load-time limit stays detectable with errors.As.
		return fmt.Errorf("%s: %w", prefix, e)
	}
	return fmt.Errorf("%s: %v", prefix, r)
}

// ---- cached loading ----
//
// A host may ship no Go toolchain: a production VM would embed the type
// information of the stdlib (and the SSA of the interpreted stdlib packages).
// LoadCached measures the per-plugin cost in that situation: the plugin is
// parsed and type-checked against already loaded packages and added to the
// existing SSA program; only its own code is compiled.

var cachedSeq atomic.Int64

// LoadCached loads the plugin in dir again, reusing p's program and packages.
func (p *Plugin) LoadCached(dir string, opts Options) (*Plugin, error) {
	return p.loadCached(dir, &opts)
}

// loadCached is LoadCached with the options by pointer (see loadImpl).
func (p *Plugin) loadCached(dir string, opts *Options) (q *Plugin, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicErr("vm cached load", r)
		}
	}()
	t0 := time.Now()
	byPath := map[string]*types.Package{}
	for _, sp := range p.M.prog.AllPackages() {
		byPath[sp.Pkg.Path()] = sp.Pkg
	}
	fset := p.M.prog.Fset
	// The files are read with the rule of the module files (openModuleFile):
	// a symbolic link is followed only when its target is a regular file
	// below dir (without symbolic links), as in a Kubernetes ConfigMap
	// volume. Nothing is read outside dir.
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		name := filepath.Join(dir, e.Name())
		file, err := openModuleFile(realDir, name)
		if file == nil && err == nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		src, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if opts.IgnoreDuplicateDecls {
		dropDupDecls(fset, files, func(path string) string {
			if tp := byPath[path]; tp != nil {
				return tp.Name()
			}
			return ""
		})
	}
	path := fmt.Sprintf("%s_cached%d", p.Pkg.Pkg.Path(), cachedSeq.Add(1))
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Implicits: map[ast.Node]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Scopes: map[ast.Node]*types.Scope{},
		Instances: map[*ast.Ident]types.Instance{}, FileVersions: map[*ast.File]string{},
	}
	conf := types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
		if tp, ok := byPath[path]; ok {
			return tp, nil
		}
		return nil, fmt.Errorf("package %s not in cache", path)
	})}
	tp, err := conf.Check(path, fset, files, info)
	if err != nil {
		return nil, err
	}
	csrc := []sourcePkg{{fset: fset, types: tp, files: files, info: info}}
	if err := opts.check(csrc); err != nil {
		return nil, err
	}
	cembeds, err := embedsOf(csrc, opts.loadLimits())
	if err != nil {
		return nil, &LoadError{Msg: err.Error()}
	}
	clinks := linknamesOf(csrc)
	for k, v := range p.M.linknames {
		if clinks == nil {
			clinks = map[*types.Func]string{}
		}
		clinks[k] = v
	}
	for k, v := range p.M.embeds {
		if cembeds == nil {
			cembeds = map[*types.Var]embedValue{}
		}
		cembeds[k] = v
	}
	t1 := time.Now()
	sp := p.M.prog.CreatePackage(tp, files, info, false)
	sp.Build()
	dropSyntax(csrc) // after the checks and the build (shrink.go)
	t2 := time.Now()
	interp := map[*types.Package]bool{tp: true}
	for k, v := range p.M.interp {
		interp[k] = v
	}
	// The VM reflect is on when the new package imports reflect, or when it
	// was on for p (then one of p's interpreted packages imports reflect).
	newM := func() *Machine {
		m := opts.newMachine(p.M.prog, interp, func(q *types.Package) bool { return q == tp || p.M.ownReflect })
		m.FieldTrack = p.M.FieldTrack // same program, same build configuration
		m.embeds = cembeds
		m.linknames = clinks
		return m
	}
	q = &Plugin{Pkg: sp, LoadTime: t1.Sub(t0), BuildTime: t2.Sub(t1)}
	if err := opts.preflight(q, newM, func(q *types.Package) bool { return q == tp }); err != nil {
		return nil, err
	}
	m := newM()
	q.M = m
	m.runInits(context.Background(), m.initOrder(sp))
	q.InitTime = time.Since(t2) - q.CheckTime
	return q, nil
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }
