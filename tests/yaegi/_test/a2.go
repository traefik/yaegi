package main

func main() {
	a := [6]int{1, 2, 3, 4, 5, 6}
	a[1] = 5
	println(a[1]) // 2
	for i, v := range a {
		println(v)
		if i == 3 {
			break
		}
	}
}

// Output:
// 5
// 1
// 5
// 3
// 4
