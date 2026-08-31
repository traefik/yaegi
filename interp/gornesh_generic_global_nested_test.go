package interp_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
)

func TestGorneshGenericGlobalNestedInitializers(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshGlobal(t, i, `func nestedIdentityGornesh[T any](v T) T { return v }`)
	evalGorneshGlobal(t, i, `
var nestedCallGornesh = nestedIdentityGornesh(nestedIdentityGornesh(41))
var arithmeticCallGornesh = nestedIdentityGornesh(40) + 2
var sliceCallsGornesh = []int{nestedIdentityGornesh(3), nestedIdentityGornesh(5)}
var mapCallsGornesh = map[string]int{
	"first": nestedIdentityGornesh(7),
	"second": nestedIdentityGornesh(9),
}
var explicitCallsGornesh = []string{
	nestedIdentityGornesh[string]("alpha"),
	nestedIdentityGornesh[string]("beta"),
}`)

	tests := []struct {
		expression string
		want       any
	}{
		{`nestedCallGornesh`, 41},
		{`arithmeticCallGornesh`, 42},
		{`sliceCallsGornesh`, []int{3, 5}},
		{`mapCallsGornesh`, map[string]int{"first": 7, "second": 9}},
		{`explicitCallsGornesh`, []string{"alpha", "beta"}},
	}
	for _, tt := range tests {
		got := evalGorneshGlobal(t, i, tt.expression).Interface()
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s = %#v, want %#v", tt.expression, got, tt.want)
		}
	}
}

func TestGorneshGenericGlobalArityDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		call string
		want string
	}{
		{"inferred missing", `arityIdentityGornesh()`, "not enough arguments"},
		{"inferred partial missing", `arityPairGornesh(1)`, "not enough arguments"},
		{"inferred extra", `arityIdentityGornesh(1, 2)`, "too many arguments"},
		{"explicit missing", `arityIdentityGornesh[int]()`, "not enough arguments"},
		{"explicit extra", `arityIdentityGornesh[int](1, 2)`, "too many arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshGlobal(t, i, `
func arityIdentityGornesh[T any](v T) T { return v }
func arityPairGornesh[T, U any](first T, second U) U { return second }`)
			_, err := i.Eval(`var arityResultGornesh = ` + tt.call)
			if err == nil {
				t.Fatalf("Eval(%q) succeeded", tt.call)
			}
			var panicErr interp.Panic
			if errors.As(err, &panicErr) {
				t.Fatalf("Eval(%q) panicked: %v", tt.call, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Eval(%q) error = %q, want substring %q", tt.call, err, tt.want)
			}
		})
	}
}

func TestGorneshGenericGlobalConversionArguments(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
	}{
		{"inferred var", `var conversionResultGornesh = conversionIdentityGornesh(conversionNumberGornesh(42))`},
		{"inferred define", `conversionResultGornesh := conversionIdentityGornesh(conversionNumberGornesh(42))`},
		{"explicit var", `var conversionResultGornesh = conversionIdentityGornesh[conversionNumberGornesh](conversionNumberGornesh(42))`},
		{"explicit define", `conversionResultGornesh := conversionIdentityGornesh[conversionNumberGornesh](conversionNumberGornesh(42))`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshGlobal(t, i, `
type conversionNumberGornesh int
func conversionIdentityGornesh[T any](value T) T { return value }`)
			evalGorneshGlobal(t, i, tt.declaration)
			got := evalGorneshGlobal(t, i, `conversionResultGornesh`).Interface()
			if want := 42; got != want {
				t.Fatalf("conversionResultGornesh = %#v, want %v", got, want)
			}
		})
	}
}

func TestGorneshGenericGlobalConversionDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		call string
		want string
	}{
		{"inferred missing conversion argument", `conversionIdentityGornesh(conversionNumberGornesh())`, "missing argument in conversion"},
		{"explicit extra conversion argument", `conversionIdentityGornesh[conversionNumberGornesh](conversionNumberGornesh(1, 2))`, "too many arguments in conversion"},
		{"inferred invalid conversion", `conversionIdentityGornesh(conversionNumberGornesh("invalid"))`, "cannot convert"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshGlobal(t, i, `
type conversionNumberGornesh int
func conversionIdentityGornesh[T any](value T) T { return value }`)
			_, err := i.Eval(`var conversionResultGornesh = ` + tt.call)
			if err == nil {
				t.Fatalf("Eval(%q) succeeded", tt.call)
			}
			var panicErr interp.Panic
			if errors.As(err, &panicErr) {
				t.Fatalf("Eval(%q) panicked: %v", tt.call, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Eval(%q) error = %q, want substring %q", tt.call, err, tt.want)
			}
		})
	}
}
