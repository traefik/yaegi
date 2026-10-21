package interp_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/traefik/yaegi/interp"
)

type operatorBatch struct {
	Rows [][]interface{}
}

func TestOperatorAssignmentToBinaryInterfaceSlice(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		input, want      interface{}
	}{
		{"string addition", `b.Rows[0][0].(string) + "!"`, "a", "a!"},
		{"integer addition", `b.Rows[0][0].(int) + 2`, 3, 5},
		{"integer negation", `-b.Rows[0][0].(int)`, 3, -3},
		{"integer complement", `^b.Rows[0][0].(int)`, 3, -4},
		{"boolean negation", `!b.Rows[0][0].(bool)`, true, false},
	} {
		for _, target := range []struct{ name, prefix, destination string }{
			{"direct", "", "b.Rows[0][0]"},
			{"row alias", "row := b.Rows[0]", "row[0]"},
		} {
			t.Run(test.name+"/"+target.name, func(t *testing.T) {
				i := interp.New(interp.Options{})
				if err := i.Use(interp.Exports{"repro/repro": {
					"Batch": reflect.ValueOf((*operatorBatch)(nil)),
				}}); err != nil {
					t.Fatal(err)
				}
				source := fmt.Sprintf(`package p
import "repro"
func Apply(b *repro.Batch) *repro.Batch {
 %s
 %s = %s
 return b
}`, target.prefix, target.destination, test.expression)
				if _, err := i.Eval(source); err != nil {
					t.Fatal(err)
				}
				value, err := i.Eval("p.Apply")
				if err != nil {
					t.Fatal(err)
				}
				batch := &operatorBatch{Rows: [][]interface{}{{test.input}}}
				got := value.Interface().(func(*operatorBatch) *operatorBatch)(batch)
				if got != batch {
					t.Errorf("returned batch = %p, want %p", got, batch)
				}
				if got := batch.Rows[0][0]; !reflect.DeepEqual(got, test.want) {
					t.Errorf("stored value = %#v, want %#v", got, test.want)
				}
			})
		}
	}
}

func TestOperatorAssignmentToBlank(t *testing.T) {
	for _, expression := range []string{"n + 1", "-n", "!(n == 3)"} {
		t.Run(expression, func(t *testing.T) {
			i := interp.New(interp.Options{})
			_, err := i.Eval(fmt.Sprintf(`package p
func Discard() int {
 n := 3
 _ = %s
 return n
}`, expression))
			if err != nil {
				t.Fatal(err)
			}
			value, err := i.Eval("p.Discard()")
			if err != nil {
				t.Fatal(err)
			}
			if got := value.Interface(); got != 3 {
				t.Errorf("Discard() = %v, want 3", got)
			}
		})
	}
}
