package main

func main() {
	var c any = int64(1)
	q := make(chan struct{})
	select {
	case q <- struct{}{}:
		println("unexpected")
	default:
		_ = c.(int64)
	}
	println("bye")
}

// Output:
// bye
