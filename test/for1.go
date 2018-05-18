package main

func main() {
	i := 0
	for i < 10 {
		if i > 4 {
			break
		}
		println(i)
		i++
	}
}

// Output:
// 0
// 1
// 2
// 3
// 4
