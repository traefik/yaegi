package vm

import (
	"os"
	"strings"
)

// Optimization switches, for before/after measurements:
// VM_OPT=none disables all; VM_OPT=-pool,-x disables some; default: all on.
var (
	optCtxPool    = optOn("pool")
	optTuple      = optOn("tuple")
	optFastNative = optOn("fastnative")
	optMemo       = optOn("memo")
	optBytes      = optOn("bytes")
	optSuper      = optOn("super")
	optAppendV    = optOn("appendv")   // append(s, x...) without the varargs array (appendv.go)
	optAggLit     = optOn("agglit")    // composite literals built in a register (aggv.go)
	optROSpill    = optOn("rospill")   // read-only locals as aliases of their value (aggv.go)
	optDeadLoad   = optOn("deadload")  // loads that nothing uses are not run (loads.go)
	optShallow    = optOn("shallow")   // loads consumed by a copy share their cells (loads.go)
	optPhi        = optOn("phi")       // values written into the register of their phi (phicoalesce.go)
	optBoxes      = optOn("boxes")     // static boxes of small values in interfaces (boxes.go)
	optIterPool   = optOn("iterpool")  // map and string iterators back to a pool at the end of a range (features.go)
	optEscape     = optOn("escape")    // locals that never outlive their frame reuse their cell (escape.go)
	optSwitch     = optOn("switch")    // chains of x == constant tests as one jump (switches.go)
	optMapKeys    = optOn("mapkeys")   // maps keep no second table of keys that their Go keys give back (value.go)
	optCallbacks  = optOn("callbacks") // funcs of common signatures given to native code without reflect.MakeFunc (callbacks.go)
)

func optOn(name string) bool {
	v := os.Getenv("VM_OPT")
	if v == "none" {
		return false
	}
	return !strings.Contains(","+v+",", ",-"+name+",")
}
