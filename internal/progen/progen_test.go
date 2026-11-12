package progen

import (
	"go/parser"
	"go/token"
	"testing"
)

// TestGenerate checks that the programs parse and that a seed always gives
// the same program. That they compile and run the same with go and yaegi v2 is
// TestGeneratedPrograms of cmd/runtests.
func TestGenerate(t *testing.T) {
	for seed := uint64(0); seed < 200; seed++ {
		src := Generate(seed)
		if src != Generate(seed) {
			t.Fatalf("seed %d: two programs", seed)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0); err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, src)
		}
	}
}
