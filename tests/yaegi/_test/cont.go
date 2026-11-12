package main

func main() {
	for i := 0; i < 10; i++ {
		if i < 5 {
			continue
		}
		println(i)
	}
}

// Output:
// 5
// 6
// 7
// 8
// 9
