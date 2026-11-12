package main

type Error interface {
	error
	Message() string
}

type T struct {
	Msg string
}

func (t *T) Error() string   { return t.Msg }
func (t *T) Message() string { return "message:" + t.Msg }

func newError() Error { return &T{"test"} }

func main() {
	e := newError()
	println(e.Error())
}

// Output:
// test
