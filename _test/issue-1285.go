package main

type (
	T1 struct{ Path [12]int8 }
	T2 struct{ Path *[12]int8 }
)

var (
	t11 = &T1{}
	t21 = &T2{}
)

func main() {
	b := [12]byte{}
	t12 := &T1{}
	t22 := &T2{}
	b11 := (*[len(t11.Path)]byte)(&b)
	b12 := (*[len(t12.Path)]byte)(&b)
	b21 := (*[len(t21.Path)]byte)(&b)
	b22 := (*[len(t22.Path)]byte)(&b)
	println(len(b11), len(b12), len(b21), len(b22))
}

// Output:
// 12 12 12 12
