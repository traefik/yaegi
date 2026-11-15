package vm

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// TestAggStoreLinear checks that finding the Store of each composite
// literal (aggStore, OpStoreAgg) costs the same for every literal of a
// block, wherever it is in the block. A package init that fills a big
// table (github.com/andybalholm/brotli) has thousands of literals in one
// block; a search of each literal from the start of its block made the
// compile time quadratic, and its preflight and init took seconds.
//
// The test checks the properties, not the shape of the SSA code: go/ssa
// changes the order of the instructions of a literal between versions
// (x/tools v0.51 stored all the elements after the last one, v0.52 stores
// each one right after it is built), and a host such as Traefik can select
// another x/tools than yaegi v2's go.mod. Whatever the shape:
//   - each literal is built from registers (OpMakeAgg or OpStoreAgg), not
//     in memory;
//   - aggStore looks at a few instructions per literal;
//   - the table holds the right values.
func TestAggStoreLinear(t *testing.T) {
	const n = 3000
	// The shape of kStaticDictionaryWords in brotli.
	var b strings.Builder
	b.WriteString("package p\n\ntype word struct {\n\tlen, transform byte\n\tidx uint16\n}\n\n")
	fmt.Fprintf(&b, "var table = [%d]word{\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\tword{%d, %d, %d},\n", i%255+1, i%3, i)
	}
	b.WriteString("}\n\n")
	b.WriteString("func Sum() int {\n\ts := 0\n\tfor i, w := range table {\n\t\ts += (i+1) * (int(w.len)*1000003 + int(w.transform)*1009 + int(w.idx))\n\t}\n\treturn s\n}\n")
	m, spkg := buildSrc(t, b.String())
	fn := spkg.Func("init")

	f := m.funcOf(fn)
	f.ensure()
	lits := 0
	for _, in := range f.code {
		if in.Op == OpMakeAgg || in.Op == OpStoreAgg {
			lits++
		}
	}
	if lits != n {
		t.Fatalf("%d literals built from registers (OpMakeAgg, OpStoreAgg), want %d", lits, n)
	}

	// aggStore looks at a few instructions per literal, not at the
	// instructions before it.
	c := &compiler{m: m, f: &Func{m: m, ssa: fn}, regs: map[ssa.Value]int32{}, blockPC: map[*ssa.BasicBlock]int{}, fused: map[ssa.Value]bool{}}
	c.skipAggregates()
	if len(c.aggs) != n {
		t.Fatalf("%d literals, want %d", len(c.aggs), n)
	}
	for _, lit := range c.aggs {
		c.aggStore(lit)
	}
	if c.aggScan > 2*n {
		t.Errorf("aggStore looked at %d instructions for %d literals, want at most %d", c.aggScan, n, 2*n)
	}

	// The values: the same sum as the compiled program.
	want := 0
	for i := 0; i < n; i++ {
		want += (i + 1) * ((i%255+1)*1000003 + (i%3)*1009 + i)
	}
	m.Call(f, nil, nil)
	if got := m.Call(m.funcOf(spkg.Func("Sum")), nil, nil).N; got != int64(want) {
		t.Errorf("Sum() = %d, want %d", got, want)
	}
}
