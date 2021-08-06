package dap

// Str returns a pointer to v.
func Str(v string) *String { return (*String)(&v) }

// Bool returns a pointer to v.
func Bool(v bool) *Boolean { return (*Boolean)(&v) }

// Int returns a pointer to v.
func Int(v int) *Integer { return (*Integer)(&v) }

// Num returns a pointer to v.
func Num(v float64) *Number { return (*Number)(&v) }
