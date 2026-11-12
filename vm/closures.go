package vm

// One object per closure.
//
// OpMakeClosure made two objects: the Closure and the slice of its free
// variables. A closure with up to 4 free variables is now one object, the
// Closure followed by an array that its Env slice points to (the Env of a
// closure is never resized or written after OpMakeClosure fills it).

// closureN are a Closure and the array of its free variables in one
// object.
type (
	closure1 struct {
		Closure
		env [1]Value
	}
	closure2 struct {
		Closure
		env [2]Value
	}
	closure3 struct {
		Closure
		env [3]Value
	}
	closure4 struct {
		Closure
		env [4]Value
	}
)

// newClosure returns a closure of fn with n free variables, to fill.
func newClosure(fn *Func, n int) *Closure {
	switch n {
	case 0:
		return &Closure{Fn: fn, Env: []Value{}}
	case 1:
		c := &closure1{Closure: Closure{Fn: fn}}
		c.Env = c.env[:]
		return &c.Closure
	case 2:
		c := &closure2{Closure: Closure{Fn: fn}}
		c.Env = c.env[:]
		return &c.Closure
	case 3:
		c := &closure3{Closure: Closure{Fn: fn}}
		c.Env = c.env[:]
		return &c.Closure
	case 4:
		c := &closure4{Closure: Closure{Fn: fn}}
		c.Env = c.env[:]
		return &c.Closure
	}
	return &Closure{Fn: fn, Env: make([]Value, n)}
}
