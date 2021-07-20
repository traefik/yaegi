package dap

func Str(v string) *String  { return (*String)(&v) }
func Bool(v bool) *Boolean  { return (*Boolean)(&v) }
func Int(v int) *Integer    { return (*Integer)(&v) }
func Num(v float64) *Number { return (*Number)(&v) }
