package generic

func Hello[T comparable](v T) *T {
	return &v
}
