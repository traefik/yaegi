package underscore

type Underscore interface {
	Call(_ string, args ...[]any) (v any, _ error)
}
