package main

import "strconv"

type Stringer interface{ String() string }

type T int

type MyErr struct{ msg string }

func (e MyErr) Error() string { return "myerr: " + e.msg }

func (t T) String() string { return "T(" + strconv.Itoa(int(t)) + ")" }

func describe(x any) string {
	switch v := x.(type) {
	case Stringer:
		return "stringer:" + v.String()
	case int:
		return "int"
	case error:
		return "error:" + v.Error()
	}
	return "other"
}

func main() {
	println(describe(T(5)), describe(7), describe(MyErr{"x"}))
}
