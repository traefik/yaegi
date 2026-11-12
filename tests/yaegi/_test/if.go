package main

func main() {
	if a := f(); a > 0 {
		println(a)
	}
}

func f() int { return 1 }

// Output:
// 1
