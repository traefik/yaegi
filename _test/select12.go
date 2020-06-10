package main

type S struct {
	q chan struct{}
}

func (s *S) Send() {
	select {
	case s.q <- struct{}{}:
		println("sent")
	default:
		println("unexpected")
	}
}
func main() {
	s := &S{q: make(chan struct{}, 1)}
	s.Send()
	println("bye")
}

// Output:
// sent
// bye
