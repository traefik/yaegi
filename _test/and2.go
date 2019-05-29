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
	println(cl, " ", ct, " ", ce)
	if ce == "" && (cl == 0 || cl > 1000) && (ct == "" || f()) {
		println("ok")
	}
}

// Output:
// 0   some text
// in f
// ok
