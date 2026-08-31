package interp_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func TestGorneshGenericGlobalInitializers(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshGlobal(t, i, `func identityGornesh[T any](v T) T { return v }`)
	evalGorneshGlobal(t, i, `func pairGornesh[T any](left, right T) (T, T) { return left, right }`)

	tests := []struct {
		declaration string
		expression  string
		want        any
	}{
		{`var inferredGornesh = identityGornesh(42)`, `inferredGornesh`, 42},
		{`var explicitGornesh = identityGornesh[string]("explicit")`, `explicitGornesh`, "explicit"},
		{`var typedGornesh int = identityGornesh(44)`, `typedGornesh`, 44},
	}
	for _, tt := range tests {
		evalGorneshGlobal(t, i, tt.declaration)
		got := evalGorneshGlobal(t, i, tt.expression).Interface()
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s = %#v, want %#v", tt.expression, got, tt.want)
		}
	}

	evalGorneshGlobal(t, i, `var pairLeftGornesh, pairRightGornesh = pairGornesh(46, 47)`)
	for name, want := range map[string]int{"pairLeftGornesh": 46, "pairRightGornesh": 47} {
		if got := evalGorneshGlobal(t, i, name).Interface(); got != want {
			t.Fatalf("%s = %#v, want %d", name, got, want)
		}
	}
}

func TestGorneshGenericGlobalInitializerBeforeFunction(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshGlobal(t, i, `var forwardGornesh = forwardIdentityGornesh(47)
func forwardIdentityGornesh[T any](v T) T { return v }`)

	if got := evalGorneshGlobal(t, i, `forwardGornesh`).Interface(); got != 47 {
		t.Fatalf("forward generic initializer = %#v, want 47", got)
	}
}

func TestGorneshGenericGlobalHookPreservesBuiltinInitializer(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshGlobal(t, i, `var builtinSliceGornesh = make([]int, 1)`)

	if got := evalGorneshGlobal(t, i, `len(builtinSliceGornesh)`).Interface(); got != 1 {
		t.Fatalf("builtin global initializer length = %#v, want 1", got)
	}
}

func TestGorneshTopLevelTypeSwitchGuard(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshGlobal(t, i, `var typeSwitchInputGornesh any = 42`)
	evalGorneshGlobal(t, i, `var typeSwitchResultGornesh int`)
	evalGorneshGlobal(t, i, `switch v := typeSwitchInputGornesh.(type) {
	case int:
		typeSwitchResultGornesh = v
	default:
		typeSwitchResultGornesh = -1
}`)

	if got := evalGorneshGlobal(t, i, `typeSwitchResultGornesh`).Interface(); got != 42 {
		t.Fatalf("type switch result = %#v, want 42", got)
	}
	if _, err := i.Eval(`v`); err == nil || !strings.Contains(err.Error(), "undefined: v") {
		t.Fatalf("type switch guard leaked or returned wrong error: %v", err)
	}
}

func TestGorneshIdenticalBinaryPackageReimport(t *testing.T) {
	tests := map[string]struct {
		importSource string
		use          string
	}{
		"default": {importSource: `import "fmt"`, use: `fmt.Sprintf("%d", 42)`},
		"alias":   {importSource: `import formatGornesh "fmt"`, use: `formatGornesh.Sprintf("%d", 42)`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			if err := i.Use(stdlib.Symbols); err != nil {
				t.Fatal(err)
			}
			evalGorneshGlobal(t, i, tc.importSource)
			evalGorneshGlobal(t, i, tc.importSource)
			if got := evalGorneshGlobal(t, i, tc.use).Interface(); got != "42" {
				t.Fatalf("reimported package use = %#v, want %q", got, "42")
			}
		})
	}

	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	evalGorneshGlobal(t, i, `import collisionGornesh "fmt"`)
	_, err := i.Eval(`import collisionGornesh "strings"`)
	if err == nil || !strings.Contains(err.Error(), "redeclared") {
		t.Fatalf("reimport collision error = %v, want redeclaration diagnostic", err)
	}
	var panicErr interp.Panic
	if errors.As(err, &panicErr) {
		t.Fatalf("reimport collision returned interpreter panic: %v", err)
	}
	if got := evalGorneshGlobal(t, i, `collisionGornesh.Sprintf("%d", 42)`).Interface(); got != "42" {
		t.Fatalf("original package after collision = %#v, want %q", got, "42")
	}
}

func evalGorneshGlobal(t *testing.T, i *interp.Interpreter, source string) reflect.Value {
	t.Helper()
	v, err := i.Eval(source)
	if err != nil {
		t.Fatalf("Eval(%q): %v", source, err)
	}
	return v
}
