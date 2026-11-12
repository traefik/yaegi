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
func TestAggStoreLinear(t *testing.T) {
	const n = 3000
	// The shape of kStaticDictionaryWords in brotli.
	var b strings.Builder
	b.WriteString("package p\n\ntype word struct {\n\tlen, transform byte\n\tidx uint16\n}\n\n")
	fmt.Fprintf(&b, "var table = [%d]word{\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\tword{%d, %d, %d},\n", i%255+1, i%3, i)
	}
	b.WriteString("}\n")
	m, spkg := buildSrc(t, b.String())
	fn := spkg.Func("init")

	// go/ssa builds each element in its own temporary, then stores all of
	// them after the last one: each is an OpMakeAgg (its Store does not
	// follow its Load), and aggStore looks at each one.
	f := m.funcOf(fn)
	f.ensure()
	makes := 0
	for _, in := range f.code {
		if in.Op == OpMakeAgg {
			makes++
		}
	}
	if makes != n {
		t.Fatalf("%d OpMakeAgg, want %d", makes, n)
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
}
