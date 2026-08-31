package interp_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func TestGorneshMixedIncrementalCellPreservesLexicalOrder(t *testing.T) {
	i := interp.New(interp.Options{})

	value, err := i.Eval(`
var mixedOrder = []int{1}
mixedOrder = append(mixedOrder, 2)
var mixedObserved = len(mixedOrder)
mixedOrder = append(mixedOrder, 3)
mixedObserved`)
	if err != nil {
		t.Fatalf("mixed Eval: %v", err)
	}
	if !value.IsValid() || value.Kind() != reflect.Int || value.Int() != 2 {
		t.Fatalf("mixed Eval value = %v, want 2", value)
	}

	assertGorneshMixedInt(t, i, `len(mixedOrder)`, 3)
	assertGorneshMixedInt(t, i, `mixedObserved`, 2)
}

func TestGorneshMixedIncrementalCellAcceptsDeclarationAfterStatement(t *testing.T) {
	i := interp.New(interp.Options{})
	if _, err := i.Eval(`var mixedBefore = 1`); err != nil {
		t.Fatalf("setup Eval: %v", err)
	}

	if _, err := i.Eval(`mixedBefore++
func mixedLate() int { return mixedBefore + 40 }
var mixedAfter = mixedLate()`); err != nil {
		t.Fatalf("statement-first mixed Eval: %v", err)
	}

	assertGorneshMixedInt(t, i, `mixedBefore`, 2)
	assertGorneshMixedInt(t, i, `mixedAfter`, 42)
}

func TestGorneshMixedIncrementalCellSupportsSemicolonsAndNewlines(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "explicit semicolons", src: `var mixedSyntax = 1; mixedSyntax++; var mixedSyntaxCopy = mixedSyntax`},
		{name: "inserted semicolons", src: "var mixedSyntax = 1\nmixedSyntax++\nvar mixedSyntaxCopy = mixedSyntax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			if _, err := i.Eval(tt.src); err != nil {
				t.Fatalf("mixed Eval: %v", err)
			}
			assertGorneshMixedInt(t, i, `mixedSyntax`, 2)
			assertGorneshMixedInt(t, i, `mixedSyntaxCopy`, 2)
		})
	}
}

func TestGorneshMixedIncrementalCellSupportsFunctionsAndImports(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatalf("Use stdlib: %v", err)
	}

	if _, err := i.Eval(`
import "strings"
func mixedUpper(s string) string { return strings.ToUpper(s) }
func mixedFib(n int) int {
	if n < 2 { return n }
	return mixedFib(n-1) + mixedFib(n-2)
}
var mixedText string
var mixedNumber int
mixedText = mixedUpper("works")
mixedNumber = mixedFib(10)`); err != nil {
		t.Fatalf("declarations-first mixed Eval: %v", err)
	}
	value, err := i.Eval(`mixedText`)
	if err != nil {
		t.Fatalf("read mixedText: %v", err)
	}
	if !value.IsValid() || value.Kind() != reflect.String || value.String() != "WORKS" {
		t.Fatalf("mixedText = %v, want WORKS", value)
	}
	assertGorneshMixedInt(t, i, `mixedNumber`, 55)
}

func TestGorneshMixedIncrementalCellDoesNotReplay(t *testing.T) {
	i := interp.New(interp.Options{})
	if _, err := i.Eval(`var mixedRuns = 0; mixedRuns++; func() { mixedRuns++ }(); func mixedMarker() {}`); err != nil {
		t.Fatalf("mixed Eval: %v", err)
	}
	assertGorneshMixedInt(t, i, `mixedRuns`, 2)
	if _, err := i.Eval(`var mixedUnrelated = 42`); err != nil {
		t.Fatalf("unrelated Eval: %v", err)
	}
	assertGorneshMixedInt(t, i, `mixedRuns`, 2)
}

func TestGorneshMixedIncrementalCellRunsEntrypointsInLexicalPhaseOrder(t *testing.T) {
	t.Run("main before later statement", func(t *testing.T) {
		i := interp.New(interp.Options{})
		if _, err := i.Eval(`var mixedMainRuns int; func main() { mixedMainRuns++ }; mixedMainRuns += 10`); err != nil {
			t.Fatal(err)
		}
		assertGorneshMixedInt(t, i, `mixedMainRuns`, 11)
	})

	t.Run("init before later statement", func(t *testing.T) {
		i := interp.New(interp.Options{})
		if _, err := i.Eval(`var mixedInitValue = 1; func init() { mixedInitValue++ }; mixedInitValue *= 10`); err != nil {
			t.Fatal(err)
		}
		assertGorneshMixedInt(t, i, `mixedInitValue`, 20)
		for name := range i.Globals() {
			if strings.HasPrefix(name, "__yaegi_mixed_init_") {
				t.Fatalf("mixed init leaked synthetic global %q", name)
			}
		}
	})
}

func TestGorneshMixedIncrementalCellDoesNotBroadenPackageFileSyntax(t *testing.T) {
	i := interp.New(interp.Options{})
	if _, err := i.Eval(`package main
var packageSyntax = 1
func main() { packageSyntax++ }`); err != nil {
		t.Fatalf("ordinary package Eval: %v", err)
	}
	assertGorneshMixedInt(t, i, `packageSyntax`, 2)

	_, err := i.Eval(`package main; var invalidPackageSyntax = 1; invalidPackageSyntax++`)
	if err == nil || !strings.Contains(err.Error(), "expected declaration") {
		t.Fatalf("package source with statement error = %v, want declaration diagnostic", err)
	}
}

func TestGorneshMixedIncrementalCellPreservesDiagnosticPositions(t *testing.T) {
	i := interp.New(interp.Options{})
	_, err := i.Eval(`var mixedLineX = 1
mixedLineX++
var mixedLineY = 2
mixedLineY = missingMixedLine`)
	if err == nil || !strings.Contains(err.Error(), "4:14: undefined: missingMixedLine") {
		t.Fatalf("mixed source diagnostic = %v, want original line 4", err)
	}

	_, err = i.Eval("var mixedSyntaxX = 1\nmixedSyntaxX++\nvar mixedSyntaxY =")
	if err == nil || !strings.Contains(err.Error(), "3:") || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("mixed syntax diagnostic = %v, want trailing declaration on original line 3", err)
	}

	_, err = i.Eval("var mixedSyntaxA = 1\nmixedSyntaxA++\nvar mixedSyntaxB = 2\nmixedSyntaxB +")
	if err == nil || !strings.Contains(err.Error(), "4:") || strings.Contains(err.Error(), "expected declaration") {
		t.Fatalf("mixed trailing-statement diagnostic = %v, want original EOF on line 4", err)
	}

	_, err = i.Eval(`var mixedSyntaxC = 1; mixedSyntaxC++; var mixedSyntaxD = 2; mixedSyntaxD = )`)
	if err == nil || strings.Contains(err.Error(), "expected declaration") || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("mixed unmatched-closer diagnostic = %v, want the malformed trailing statement", err)
	}
}

func assertGorneshMixedInt(t *testing.T, i *interp.Interpreter, src string, want int64) {
	t.Helper()
	value, err := i.Eval(src)
	if err != nil {
		t.Fatalf("Eval(%q): %v", src, err)
	}
	if !value.IsValid() || value.Kind() != reflect.Int || value.Int() != want {
		t.Fatalf("Eval(%q) = %v, want %d", src, value, want)
	}
}
