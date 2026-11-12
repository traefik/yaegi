package main

func main() {
	i := 0
	for {
		if i > 10 {
			break
		}
		if i < 5 {
			i++
			continue
		}
		println(i)
		i++
	}
}

// Output:
// 5
// 6
// 7
// 8
// 9
// 10
