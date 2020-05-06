package main

func Bar() string {
	return
}

func main() {
	println(Bar())
}

// Error:
// 4:2: not enough arguments to return
