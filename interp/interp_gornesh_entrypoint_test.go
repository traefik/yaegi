package interp_test

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/interp"
)

func TestGorneshExplicitMainRunsOnlyWhenDeclaredOrCalled(t *testing.T) {
	i := interp.New(interp.Options{})

	if _, err := i.Eval(`var mainRuns int
func main() { mainRuns++ }`); err != nil {
		t.Fatal(err)
	}
	if got := evalInt(t, i, "mainRuns"); got != 1 {
		t.Fatalf("main ran again during unrelated Eval: got %d, want 1", got)
	}

	if got := evalInt(t, i, "main(); mainRuns"); got != 2 {
		t.Fatalf("explicit main call count: got %d, want 2", got)
	}
	if got := evalInt(t, i, "mainRuns"); got != 2 {
		t.Fatalf("main replayed after explicit call: got %d, want 2", got)
	}
}

func TestGorneshTopLevelIIFERunsOnceAndReturnsValue(t *testing.T) {
	i := interp.New(interp.Options{})

	if _, err := i.Eval("var iifeRuns int"); err != nil {
		t.Fatal(err)
	}
	if got := evalInt(t, i, `func() int { iifeRuns++; return 42 }()`); got != 42 {
		t.Fatalf("IIFE result: got %d, want 42", got)
	}
	if got := evalInt(t, i, "iifeRuns"); got != 1 {
		t.Fatalf("IIFE replayed during later Eval: got %d, want 1", got)
	}
}

func TestGorneshTopLevelIIFEDoesNotClobberExplicitMain(t *testing.T) {
	i := interp.New(interp.Options{})

	if _, err := i.Eval(`var mainRuns, iifeRuns int
func main() { mainRuns++ }`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`func() { iifeRuns++ }()`); err != nil {
		t.Fatal(err)
	}
	if got := evalInt(t, i, "main(); mainRuns*10+iifeRuns"); got != 21 {
		t.Fatalf("main/IIFE state: got %d, want 21", got)
	}
}

func evalInt(t *testing.T, i *interp.Interpreter, src string) int {
	t.Helper()
	v, err := i.Eval(src)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsValid() {
		t.Fatalf("Eval(%q) returned no value", src)
	}
	if v.Kind() != reflect.Int {
		t.Fatalf("Eval(%q) returned %s, want int", src, v.Kind())
	}
	return int(v.Int())
}
