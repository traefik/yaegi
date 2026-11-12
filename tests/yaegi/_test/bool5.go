package main

func main() {
	var b bool
	m := &b

	if *m {
		println(0)
	} else {
		println(1)
	}
}

// Output:
// 1
