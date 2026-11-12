package interp

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/stdlib"
)

// testUpstream1499 evaluates src and compares the interpreter output with the
// expected output of the same program compiled by the native Go compiler.
func testUpstream1499(t *testing.T, src, expected string) {
	t.Helper()
	var out bytes.Buffer
	i := New(Options{Stdout: &out})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(src); err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if got := out.String(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestUpstreamIssue1499 checks that a method expression on an interpreted
// type (for example (*T).M) produces a func value taking the receiver as
// first argument, instead of a method value bound to an empty receiver.
func TestUpstreamIssue1499(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c *Client) set() { c.Count++ }

func onItem(c *Client, fn func(*Client)) { fn(c) }

func main() {
	c := &Client{}
	onItem(c, (*Client).set)
	fmt.Println("expr form count:", c.Count)
}
`, "expr form count: 1\n")
}

// TestUpstreamIssue1499ValueBase checks the method expression form on a value
// type base, for a value receiver method.
func TestUpstreamIssue1499ValueBase(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c Client) incr(by int) int { c.Count += by; return c.Count }

func main() {
	c := Client{Count: 7}
	f := Client.incr
	fmt.Println("f:", f(c, 3))
	fmt.Println("count unchanged:", c.Count)
	fmt.Println("direct:", Client.incr(c, 5))
}
`, "f: 10\ncount unchanged: 7\ndirect: 12\n")
}

// TestUpstreamIssue1499PointerBaseValueReceiver checks a value receiver method
// expressed on a pointer type base: the signature takes *T and the method
// operates on a copy of the pointed value.
func TestUpstreamIssue1499PointerBaseValueReceiver(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c Client) incr() { c.Count++ }

func onItem(c *Client, fn func(*Client)) { fn(c) }

func main() {
	c := &Client{Count: 1}
	onItem(c, (*Client).incr)
	fmt.Println("count unchanged:", c.Count)
}
`, "count unchanged: 1\n")
}

// TestUpstreamIssue1499ArgsReturns checks a method expression with arguments
// and results, stored in a variable and called directly.
func TestUpstreamIssue1499ArgsReturns(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c *Client) add(n int) int {
	c.Count += n
	return c.Count
}

func main() {
	var f func(*Client, int) int = (*Client).add
	c := &Client{}
	fmt.Println("var form:", f(c, 3))
	fmt.Println("direct form:", (*Client).add(c, 4))
	fmt.Println("count:", c.Count)
}
`, "var form: 3\ndirect form: 7\ncount: 7\n")
}

// TestUpstreamIssue1499Promoted checks method expressions on promoted methods
// of an embedded field, with value and pointer receivers.
func TestUpstreamIssue1499Promoted(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Base struct{ N int }

func (b Base) hi() string { return fmt.Sprintf("hi %d", b.N) }

func (b *Base) inc() { b.N++ }

type Outer struct {
	Base
	M int
}

func main() {
	o := Outer{Base{1}, 2}
	f := Outer.hi
	fmt.Println("value:", f(o))
	g := (*Outer).inc
	g(&o)
	fmt.Println("ptr:", o.N)
	// A method value of the same method compiled before must not change the
	// method expression behavior.
	h := o.hi
	fmt.Println("method value:", h())
	fmt.Println("expr again:", Outer.hi(o))
}
`, "value: hi 1\nptr: 2\nmethod value: hi 2\nexpr again: hi 2\n")
}

// TestUpstreamIssue1499PromotedDefer checks a promoted method expression used
// as the call of a deferred statement: the outer receiver must be projected
// onto the embedded method receiver when the deferred call runs.
func TestUpstreamIssue1499PromotedDefer(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Base struct{ N int }

func (b *Base) bump() { b.N++ }

func (b Base) hi() string { return fmt.Sprintf("hi %d", b.N) }

type Outer struct {
	Base
	M int
}

func main() {
	o := Outer{Base{1}, 2}
	defer (*Outer).bump(&o)
	defer fmt.Println("deferred hi:", Outer.hi(o))
	fmt.Println("body:", o.N)
}
`, "body: 1\ndeferred hi: hi 1\n")
}

// TestUpstreamIssue1499Variadic checks a method expression of a variadic
// method.
func TestUpstreamIssue1499Variadic(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c *Client) add(ns ...int) int {
	for _, n := range ns {
		c.Count += n
	}
	return c.Count
}

func main() {
	f := (*Client).add
	c := &Client{}
	fmt.Println(f(c, 1, 2, 3))
	fmt.Println(c.Count)
}
`, "6\n6\n")
}

// TestUpstreamIssue1499FuncValueContainers checks that method expression
// values can be stored in composite types and called back.
func TestUpstreamIssue1499FuncValueContainers(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c *Client) set() { c.Count++ }

type Holder struct {
	F func(*Client)
}

func main() {
	h := Holder{F: (*Client).set}
	c := &Client{}
	h.F(c)
	s := []func(*Client){(*Client).set}
	s[0](c)
	m := map[string]func(*Client){"set": (*Client).set}
	m["set"](c)
	fmt.Println("count:", c.Count)
}
`, "count: 3\n")
}

// TestUpstreamIssue1499HostBoundary checks that a method expression value can
// cross the interpreter to host function boundary and back, keeping a usable
// receiver-first func value.
func TestUpstreamIssue1499HostBoundary(t *testing.T) {
	var out bytes.Buffer
	i := New(Options{Stdout: &out})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(Exports{
		"host/host": {
			"RoundTrip": reflect.ValueOf(func(v interface{}) interface{} { return v }),
		},
	}); err != nil {
		t.Fatal(err)
	}
	src := `package main

import (
	"fmt"
	"host"
)

type Client struct{ Count int }

func (c *Client) twice(n int) int { c.Count += n; return c.Count * 2 }

func main() {
	f := host.RoundTrip((*Client).twice).(func(*Client, int) int)
	c := &Client{}
	fmt.Println("roundtrip:", f(c, 5))
	fmt.Println("count:", c.Count)
}
`
	if _, err := i.Eval(src); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "roundtrip: 10\ncount: 5\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestUpstreamIssue1499MethodValueUnchanged guards that method values
// (receiver bound at evaluation time) still behave as before.
func TestUpstreamIssue1499MethodValueUnchanged(t *testing.T) {
	testUpstream1499(t, `package main

import "fmt"

type Client struct{ Count int }

func (c *Client) set() { c.Count++ }

func onItem(c *Client, fn func()) { fn() }

func main() {
	c := &Client{}
	fn := c.set
	onItem(c, fn)
	fmt.Println("value form count:", c.Count)
	fn()
	fmt.Println("still bound:", c.Count)
}
`, "value form count: 1\nstill bound: 2\n")
}

// TestUpstreamIssue1499InvalidExpr checks that invalid method expressions are
// still rejected at compile time.
func TestUpstreamIssue1499InvalidExpr(t *testing.T) {
	for _, test := range []struct {
		desc, src, errContains string
	}{
		{
			desc: "pointer receiver method on value type base",
			src: `package main

type Client struct{ Count int }

func (c *Client) set() { c.Count++ }

func main() {
	c := Client{}
	f := Client.set
	f(&c)
}
`,
			errContains: "invalid method expression main.Client.set (needs pointer receiver (*main.Client).set)",
		},
		{
			desc: "method expression signature mismatch",
			src: `package main

type Client struct{ Count int }

func (c *Client) set() { c.Count++ }

func main() {
	var f func() = (*Client).set
	c := &Client{}
	f(c)
	_ = c
}
`,
			errContains: "cannot use type func(*main.Client) as type func()",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			i := New(Options{})
			if err := i.Use(stdlib.Symbols); err != nil {
				t.Fatal(err)
			}
			_, err := i.Eval(test.src)
			if err == nil {
				t.Fatalf("got no error, want %q", test.errContains)
			}
			if !strings.Contains(err.Error(), test.errContains) {
				t.Errorf("got error %q, want it to contain %q", err, test.errContains)
			}
		})
	}
}

// TestUpstreamIssue1499BinaryTypeMismatch guards that a receiver-signature
// mismatch on a binary type method expression is still rejected at compile
// time.
func TestUpstreamIssue1499BinaryTypeMismatch(t *testing.T) {
	i := New(Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	src := `package main

import "strings"

func main() {
	var f func(string) (int, error) = (*strings.Builder).WriteString
	_ = f
}
`
	_, err := i.Eval(src)
	if err == nil {
		t.Fatal("got no error, want a type mismatch error")
	}
	if !strings.Contains(err.Error(), "cannot use type") {
		t.Errorf("got error %q, want a type mismatch", err)
	}
}
