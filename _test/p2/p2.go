package p2

type I interface {
	isI()
}

type T struct{}

func (t *T) isI() {}
