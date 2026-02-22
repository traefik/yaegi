package generic

func Hello[T comparable](v T) *T { //yaegi:add
	return &v
}
