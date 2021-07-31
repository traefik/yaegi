package main

type Enabler interface {
	Enabled() bool
}

type Logger struct {
	core Enabler
}

func (log *Logger) GetCore() Enabler { return log.core }

type T struct{}

func (t *T) Enabled() bool { return true }

func main() {
	base := &Logger{&T{}}
	println(base.GetCore().Enabled())
}

// Output:
// true
