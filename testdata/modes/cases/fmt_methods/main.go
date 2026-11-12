// fmt: error, GoStringer, Formatter on interpreted types; which method
// wins for which verb; nested use.
package main

import (
	"errors"
	"fmt"
	"strconv"
)

type MyErr struct{ Code int }

func (e MyErr) Error() string { return "myerr " + strconv.Itoa(e.Code) }

type PErr struct{ Msg string }

func (e *PErr) Error() string { return "perr: " + e.Msg }

type Both struct{}

func (Both) Error() string  { return "as error" }
func (Both) String() string { return "as string" }

type Go struct{ A int }

func (g Go) GoString() string { return "Go{A:" + strconv.Itoa(g.A) + "}" }
func (g Go) String() string   { return "go-string" }

type Money struct{ C int }

func (m Money) Format(f fmt.State, verb rune) {
	w, wok := f.Width()
	p, pok := f.Precision()
	fmt.Fprintf(f, "Money(%c w=%d/%v p=%d/%v plus=%v sharp=%v minus=%v: %d)", verb, w, wok, p, pok, f.Flag('+'), f.Flag('#'), f.Flag('-'), m.C)
}

type Box struct {
	E   error
	G   Go
	M   Money
	Any any
}

func main() {
	fmt.Println(MyErr{1}, &PErr{"x"}, Both{})
	fmt.Printf("%v|%s|%q|%d|%x\n", MyErr{2}, MyErr{2}, MyErr{2}, MyErr{2}, MyErr{2})
	fmt.Printf("%v|%#v|%s|%+v\n", Go{3}, Go{3}, Go{3}, Go{3})
	fmt.Printf("%v|%+v|%#v|%5.2s|%-d|%x\n", Money{4}, Money{4}, Money{4}, Money{4}, Money{4}, Money{4})
	b := Box{E: MyErr{5}, G: Go{6}, M: Money{7}, Any: MyErr{11}}
	fmt.Printf("%v\n%+v\n%#v\n", b, b, b)
	fmt.Println([]error{MyErr{8}, &PErr{"s"}}, []Go{{1}}, []Money{{2}})
	fmt.Printf("%#v\n", []Go{{9}})
	err := fmt.Errorf("wrapped: %w", MyErr{10})
	fmt.Println(err)
	fmt.Printf("%v %T\n", errors.Unwrap(err), errors.Unwrap(err))
	var e error = &PErr{"iface"}
	fmt.Printf("%v %T %#v\n", e, e, e.(*PErr).Msg)
}
