package d1

type T struct {
	Name string
}

func (t *T) F() { println(t.Name) }

func NewT(s string) *T { return &T{s} }
