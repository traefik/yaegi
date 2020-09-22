package main

type T struct{ I int }

func main() {
	t := []*T{}
	s := []int{1, 2}
	for _, e := range s {
		x := &T{I: e}
		t = append(t, x)
	}
	println(t[0].I, t[1].I)
}

// Output:
// 1 2
