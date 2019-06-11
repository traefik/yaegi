package main

const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func main() {
	for i, r := range itoa64 {
		if r == '1' {
			println(i)
		}
	}
}

// Output:
// 3
