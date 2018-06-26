package main

func main() {
	i := 0
	for {
		if i > 10 {
			break
		}
		i++
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
// 10
// 11
