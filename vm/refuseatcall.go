package vm

// Options.RefuseAtCall: the refused functions refuse at call time instead of
// at load time.
//
// Without the option, the load-time check (restrict.go) refuses a plugin
// that names a function of refusedFuncs or imports a package of
// refusedImports, even when the code never calls it. Big dependency graphs
// often have such code in branches that a plugin never runs: a library that
// can also run as a command (caddy and its cmd package: os/signal.Ignore,
// runtime/debug.SetMemoryLimit), a logger that can write to journald
// (go-systemd: (*net.UnixConn).WriteMsgUnix), a cluster library with
// service-discovery plugins loaded with plugin.Open (olric), a debug API
// that serves net/http/pprof (souin).
//
// With the option, such a plugin loads, and a call of a refused function
// panics with a *RefusedError, without calling it:
//
//   - the functions of refusedFuncs get a stub (refusedCallHook); their
//     methods are already refused at run time (refusedmeth.go), for static
//     calls too;
//   - the packages of refusedAtCallImports may be imported, and each of their
//     functions and methods is a stub. Their package init does not run in the
//     plugin: the VM never runs the init of a native package (net/http/pprof
//     registers its routes on the host's http.DefaultServeMux only when the
//     host itself imports it).
//
// The other refused imports stay refused at load time: os/exec has
// Options.Exec (ExecMock loads the plugin and starts no process: host.Options
// sets it with RefuseAtCall), and
// go/types, go/build, go/importer and net/http/cgi have variables or types
// that a plugin uses without a call. The function the plugin gets is never
// the real one, so the option gives the plugin no more access to the host
// than it has without it. The cost is for the plugin: the error comes in the
// middle of a request instead of at load time, as with StubAssembly.

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// refusedAtCallImports are the packages of refusedImports whose functions
// become stubs with Options.RefuseAtCall.
var refusedAtCallImports = map[string]bool{"plugin": true, "net/http/pprof": true}

// refusedCallHook returns, for a machine with Options.RefuseAtCall, a stub of
// the native function f when it is refused: a function of refusedFuncs (not
// a method: refusedmeth.go has them), or a function or method of a package of
// refusedAtCallImports. nil otherwise.
//
// With Options.Mounts, the functions and methods of mountRefused, which
// open a path by themselves, get a stub too (mount.go).
func (m *Machine) refusedCallHook(f *ssa.Function) NativeHook {
	if !m.refuseAtCall && m.mounts == nil || f.Pkg == nil || m.isInterp(f) {
		return nil
	}
	obj, ok := f.Object().(*types.Func)
	if !ok {
		return nil
	}
	name := obj.Origin().FullName()
	if m.mounts != nil && mountRefused[name] {
		what := name + " (it opens a path outside Options.Mounts)"
		return func(*Machine, []Value) Value {
			panic(&RefusedError{What: what})
		}
	}
	if !m.refuseAtCall {
		return nil
	}
	switch {
	case refusedAtCallImports[f.Pkg.Pkg.Path()]:
	case f.Signature.Recv() != nil || !refusedFuncs[name]:
		return nil
	case name == "os.StartProcess" && m.execMode != ExecRefused:
		return nil // Options.Exec: osexec.go
	}
	what := name + " (Options.RefuseAtCall loaded the plugin)"
	return func(*Machine, []Value) Value {
		panic(&RefusedError{What: what})
	}
}
