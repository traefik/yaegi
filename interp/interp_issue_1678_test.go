package interp

import (
	"go/token"
	"testing"
)

// TestEvalFileSetGrowth checks that one Eval registers one file in interp.fset.
// firstToken and buildOk used to add two more that nothing referenced again.
func TestEvalFileSetGrowth(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`var price float64`); err != nil {
		t.Fatal(err)
	}

	files := func() int {
		n := 0
		i.FileSet().Iterate(func(*token.File) bool { n++; return true })
		return n
	}

	const evals = 100
	before := files()
	for k := 0; k < evals; k++ {
		if _, err := i.Eval(`price = 1.0`); err != nil {
			t.Fatal(err)
		}
	}

	if got := files() - before; got > evals {
		t.Errorf("file set grew by %d files over %d Eval calls, want at most %d", got, evals, evals)
	}
}
