package main

func main() {
	for i := 2; ; i++ {
		println(i)
		if i > 3 {
			break
		}
	}
}

// Output:
// 2
// 3
// 4
