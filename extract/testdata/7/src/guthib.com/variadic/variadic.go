package variadic

type Variadic interface {
	Call(method string, args ...interface{}) (interface{}, error)
}
