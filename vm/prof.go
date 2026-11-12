package vm

import (
	"fmt"
	"io"
	"os"
	"sort"
)

// VM-level profile (VM_PROFILE=1): executed instructions per opcode and per
// function. Single-threaded use only (benchmark loop).
var profOps = os.Getenv("VM_PROFILE") != ""

// OpCount counts executed instructions per opcode.
var OpCount [256]int64

var opNames = map[Op]string{}

func init() {
	names := []string{"Nop", "Mov", "Jmp", "If", "Ret", "AddI", "SubI", "MulI", "DivI", "RemI", "DivU", "RemU", "AndI", "OrI", "XorI", "AndNotI", "ShlI", "ShrI", "ShrU", "EqI", "NeI", "LtI", "LeI", "GtI", "GeI", "LtU", "LeU", "GtU", "GeU", "AddF", "SubF", "MulF", "DivF", "LtF", "LeF", "GtF", "GeF", "EqF", "AddS", "EqS", "NeS", "LtS", "EqV", "NeV", "NegI", "NegF", "Not", "Compl", "Trunc", "I2F", "F2I", "Str2Bytes", "Bytes2Str", "Rune2Str", "Call", "CallClosure", "CallNative", "Invoke", "MakeClosure", "Alloc", "Load", "Store", "FieldAddr", "Field", "IndexAddr", "Index", "Slice", "MakeSlice", "Len", "Cap", "Append", "Copy", "MakeInterface", "TypeAssert", "Extract", "MakeMap", "MapUpdate", "Lookup", "Println", "Panic", "Defer", "RunDefers", "Recover", "Go", "MakeChan", "Send", "Recv", "Close", "Select", "Range", "Next", "Delete", "Func", "FieldLoad", "FieldStore", "IndexLoad", "IndexStore", "JEqI", "JNeI", "JLtI", "JLeI", "JGtI", "JGeI", "JLtU", "JLeU", "JGtU", "JGeU", "JEqS", "JNeS", "Str2Runes", "Runes2Str", "WrapNilChk", "GC", "CtxFunc", "AppendV", "MakeAgg", "Switch", "MovJmp", "MakeIfaceBox", "AllocLocal", "StoreAgg"}
	for i, n := range names {
		opNames[Op(i)] = n
	}
}

// ResetProfile clears the VM profile counters.
func (m *Machine) ResetProfile() {
	OpCount = [256]int64{}
	for _, f := range m.funcs {
		f.icount = 0
	}
}

// DumpProfile writes the VM profile, normalized per request.
func (m *Machine) DumpProfile(w io.Writer, requests int) {
	var total int64
	type kv struct {
		k string
		v int64
	}
	var ops []kv
	for i, c := range OpCount {
		total += c
		if c > 0 {
			name := opNames[Op(i)]
			if name == "" {
				name = fmt.Sprint(i)
			}
			ops = append(ops, kv{name, c})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].v > ops[j].v })
	r := float64(requests)
	fmt.Fprintf(w, "VM instructions per request: %.0f\n", float64(total)/r)
	for _, o := range ops[:min(25, len(ops))] {
		fmt.Fprintf(w, "  op %-14s %9.0f  %5.1f%%\n", o.k, float64(o.v)/r, 100*float64(o.v)/float64(total))
	}
	var fs []kv
	for _, f := range m.funcs {
		if f.icount > 0 {
			fs = append(fs, kv{f.ssa.String(), f.icount})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].v > fs[j].v })
	for _, f := range fs[:min(25, len(fs))] {
		fmt.Fprintf(w, "  fn %-60s %9.0f  %5.1f%%\n", f.k, float64(f.v)/r, 100*float64(f.v)/float64(total))
	}
}
