package main

func main() {
	dict := map[int32]int64{13: 733}
	for k, v := range dict {
		println(k, v)
	}
}

// Output:
// 13 733
