package vm

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// evalSession returns a session of a program (the real standard library),
// whose standard output is out.
func evalSession(t *testing.T, out *bytes.Buffer) *Session {
	t.Helper()
	s, err := NewSession(Options{Unrestricted: true, Stdout: out})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// evalValues runs src and returns its values as fmt.Sprintln writes them.
func evalValues(t *testing.T, s *Session, src string) string {
	t.Helper()
	ctx := context.Background()
	res, err := s.Eval(ctx, src)
	if err != nil {
		t.Fatalf("Eval(%q): %v", src, err)
	}
	if len(res) == 0 {
		return ""
	}
	str, err := s.Sprint(ctx)
	if err != nil {
		t.Fatalf("Sprint after %q: %v", src, err)
	}
	return strings.TrimSuffix(str, "\n")
}

// TestEvalInputs runs inputs one after the other in one session: each one
// sees the variables, functions, types and imports of the earlier ones.
func TestEvalInputs(t *testing.T) {
	var out bytes.Buffer
	s := evalSession(t, &out)
	steps := []struct{ src, want string }{
		{"1 + 2", "3"},
		{`import "strings"`, ""},
		{`strings.ToUpper("abc")`, "ABC"},
		{"x := 10", ""},
		{"func double() int { return x * 2 }", ""},
		{"double()", "20"},
		{"x = 21; double()", "42"},
		// A type and its methods in one input; its unexported fields
		// stay visible to the next inputs, also in a literal without keys.
		{"type point struct{ x, y int }\nfunc (p *point) move(d int) { p.x += d; p.y += d }", ""},
		{"p := &point{1, 2}; p.move(1)", ""},
		{"q := point{x: 7}; p.x + p.y + q.x", "12"},
		// var is a declaration in an input without statements.
		{"var a, b = 1, \"s\"", ""},
		{"func pair() (int, string) { return a, b }\npair()", "1 s"},
		// Sprintf gives a value; only Print* writes and gives none.
		{`import "fmt"`, ""},
		{`fmt.Sprintf("v=%d", 3)`, "v=3"},
		// reflect imported after the first input sees the program's types.
		{`import "reflect"`, ""},
		{"type T struct{ A int }; func (T) M() int { return 5 }", ""},
		{`v := reflect.ValueOf(T{}); v.Type().String() + " " + fmt.Sprint(v.NumMethod(), v.MethodByName("M").Call(nil)[0].Int())`, "repl.T 1 5"},
		{`import "strconv"; strconv.Atoi("12")`, "12 <nil>"},
		// A goroutine and a channel.
		{"ch := make(chan int, 1)\ngo func() { ch <- 7 }()\n<-ch", "7"},
		// x is a string now; double still uses the old x.
		{`x := "s"; x + "!"`, "s!"},
		{"double()", "42"},
	}
	for _, st := range steps {
		if got := evalValues(t, s, st.src); got != st.want {
			t.Errorf("Eval(%q) = %q, want %q", st.src, got, st.want)
		}
	}
	if _, err := s.Eval(context.Background(), `import "fmt"; fmt.Println("hello", x)`); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello s\n" {
		t.Errorf("output %q, want %q", got, "hello s\n")
	}
}

// TestEvalErrors checks the inputs that Eval refuses, and that the session
// still works after them.
func TestEvalErrors(t *testing.T) {
	s := evalSession(t, new(bytes.Buffer))
	ctx := context.Background()
	evalValues(t, s, "type T struct{ n int }\nn := 1")
	errs := []struct{ src, want string }{
		{"func (T) m() {}", "declare a type and its methods in the same input"},
		{"n := n + 1", "n is declared again in this input"},
		{"y := undefinedName", "undefined: undefinedName"},
		{`import "github.com/traefik/yaegi/v2/vm"`, "only import packages of the standard library"},
		{"1 + 2; 3", "is not used"},
		{`panic("boom")`, "boom"},
	}
	for _, e := range errs {
		_, err := s.Eval(ctx, e.src)
		if err == nil || !strings.Contains(err.Error(), e.want) {
			t.Errorf("Eval(%q): error %v, want %q", e.src, err, e.want)
		}
	}
	for _, src := range []string{"func f() {", "s := `abc", "x := []int{1,"} {
		if _, err := s.Eval(ctx, src); !errors.Is(err, ErrIncomplete) {
			t.Errorf("Eval(%q): error %v, want ErrIncomplete", src, err)
		}
	}
	if got := evalValues(t, s, "n + T{n: 2}.n"); got != "3" {
		t.Errorf("after the errors: %q, want 3", got)
	}
}

// TestEvalRestricted checks that a session with the options of a plugin
// applies its restrictions.
func TestEvalRestricted(t *testing.T) {
	s, err := NewSession(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Eval(context.Background(), `import "os/exec"; exec.Command("true")`); err == nil {
		t.Fatal("os/exec accepted in a restricted session")
	}
}
