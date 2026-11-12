package main

func main() {
	for i := 0; i < 4; i++ {
		for {
			break
		}
		if i == 1 {
			continue
		}
		println(i)
	}
}

// Output:
// 0
// 2
// 3
