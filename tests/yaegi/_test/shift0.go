package main

func main() {
	var rounds uint64
	var cost uint32 = 2
	rounds = 1 << cost
	println(rounds)
}

// Output:
// 4
