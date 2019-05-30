package main

func f() bool {
	println("in f")
	return true
}

func main() {
	var (
		cl = 0
		ct = "some text"
		ce = ""
	)
	if ce == "" && (cl == 0 || cl > 1000) && (ct == "" || f()) {
		println("ok")
	}
}

// Output:
// in f
// ok
