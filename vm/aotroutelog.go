package vm

// The print functions of log with interpreted values, in default mode.
//
// log formats its message with native fmt (fmt.Append, Appendln,
// Appendf), which sees an interpreted value as a native copy or a
// hand-written wrapper: the Format method of a plugin type was not called
// (log.Print(M{1}) printed {1}), nor String of a nested value. When the
// AOT copy of fmt is routed (aotroute.go), or in AOT mode, a print function of log whose
// arguments hold an interpreted type formats the message with the copy,
// then gives the native log function the message as one string, which it
// prints as is: Print(msg), Println(msg without its last newline),
// Printf("%s", msg). The native function keeps what it does around the
// message (the prefix and flags of the logger, the output of the plugin,
// Fatal and Panic), and Panic panics with the same string as in Go.

import (
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// logSprint is the fmt function of each print function of log.
var logSprint = map[string]string{
	"Print": "Sprint", "Println": "Sprintln", "Printf": "Sprintf",
	"Fatal": "Sprint", "Fatalln": "Sprintln", "Fatalf": "Sprintf",
	"Panic": "Sprint", "Panicln": "Sprintln", "Panicf": "Sprintf",
}

// logRouteCall returns the implementation of a call of the print function
// f of log (or method of *log.Logger) with the argument registers args,
// when the AOT copy of fmt is routed or used (nil otherwise).
func (m *Machine) logRouteCall(f *ssa.Function, args []int32) func([]Value) Value {
	if m.vr == nil || aotPkg("fmt") == nil || !m.routed["fmt"] && m.modeOf("fmt") != ModeAOT || f.Pkg == nil || f.Pkg.Pkg.Path() != "log" {
		return nil
	}
	sprint, ok := logSprint[f.Name()]
	if !ok {
		return nil
	}
	if r := f.Signature.Recv(); r != nil && r.Type().String() != "*log.Logger" {
		return nil
	}
	fmtFn, ok := aotPkg("fmt").Funcs[sprint]
	if !ok {
		return nil
	}
	s := m.nativeSite(f, args)
	idx := make([]int32, len(args))
	for i := range idx {
		idx[i] = int32(i)
	}
	own := m.nativeSite(f, idx) // the call with the message: its own registers
	n := len(args)
	str := m.rtypeOf(types.Typ[types.String])
	return func(regs []Value) Value {
		if !m.routeNeeded(regs[args[n-1]], true, "fmt") {
			return m.callNativeSite(s, regs)
		}
		vs, _ := regs[args[n-1]].R.([]Value)
		var wb []func()
		c := newConv(&wb)
		c.g.aot = true
		xs := make([]any, len(vs))
		for j, v := range vs {
			xs[j] = m.routeArg(v, c)
		}
		var in []reflect.Value
		if sprint == "Sprintf" {
			format, _ := regs[args[n-2]].R.(string)
			in = append(in, reflect.ValueOf(format))
		}
		in = append(in, reflect.ValueOf(xs))
		msg := func() string {
			defer writeBack(wb)
			defer m.flushShadows()
			return fmtFn.CallSlice(in)[0].String()
		}()
		small := make([]Value, n)
		for i, a := range args {
			small[i] = regs[a]
		}
		switch sprint {
		case "Sprintln":
			msg = strings.TrimSuffix(msg, "\n")
		case "Sprintf":
			small[n-2] = Value{R: "%s"}
		}
		small[n-1] = Value{R: []Value{{R: &Iface{T: str, V: Value{R: msg}}}}}
		return m.callNativeSite(own, small)
	}
}
