package main

func main() {
	i := 0
	//for ;i >= 0; i++ {
	for {
		if i > 5 {
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
// 5
