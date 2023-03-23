package main

type I1 interface {
	I2
	Wrap() *S3
}

type I2 interface {
	F()
}

type S2 struct {
	I2
}

func newS2(i2 I2) I1 {
	return &S2{i2}
}

type S3 struct {
	base *S2
}

func (s *S2) Wrap() *S3 {
	i2 := s
	return &S3{i2}
}

type T struct {
	name string
}

func (t *T) F() { println("in F", t.name) }

func main() {
	t := &T{"test"}
	s2 := newS2(t)
	s3 := s2.Wrap()
	s3.base.F()
}

// Output:
// in F test
