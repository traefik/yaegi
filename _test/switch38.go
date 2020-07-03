package main

func isSeparator(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}', ' ', '\t':
		return true
	}
	return false
}

func main() {
	s := "max-age=20"
	for _, c := range []byte(s) {
		println(string(c), isSeparator(c))
	}
}

// Output:
// m false
// a false
// x false
// - false
// a false
// g false
// e false
// = true
// 2 false
// 0 false
