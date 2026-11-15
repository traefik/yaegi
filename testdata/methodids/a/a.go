// Package a has unexported methods that the types of other packages
// embed: their names must not catch the methods of the same name of the
// other packages.
package a

// T has the unexported method m.
type T struct{}

func (T) m() string { return "a.T.m" }

// P has the unexported method m on its pointer.
type P struct{ N int }

func (p *P) m() string { return "a.P.m" }

// I is satisfied by the types that have the method m of package a.
type I interface{ m() string }

// Call calls the method m of package a.
func Call(i I) string { return i.m() }

// Value returns the method value i.m.
func Value(i I) func() string { return i.m }

// Defer calls i.m in a deferred call.
func Defer(i I) (s string) {
	defer func() { s = "defer " + s }()
	defer func() { s = i.m() }()
	return ""
}

// Go calls i.m in a goroutine.
func Go(i I) string {
	c := make(chan string)
	go func() { c <- i.m() }()
	return <-c
}

// Expr is the method expression I.m.
func Expr(i I) string { return I.m(i) }

// Switch tells which interfaces of package a the value has.
func Switch(v any) string {
	switch x := v.(type) {
	case I:
		return "a.I " + x.m()
	default:
		return "not a.I"
	}
}

// Gen calls the method m of package a on a type parameter.
func Gen[X I](x X) string { return x.m() }
