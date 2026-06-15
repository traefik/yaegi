package variadic

type Variadic interface {
	Call(method string, args ...[]any) (any, error)
}
