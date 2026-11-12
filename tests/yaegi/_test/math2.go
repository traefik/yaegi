package main

const c uint64 = 2

func main() {
	if c&(1<<(uint64(1))) > 0 {
		println("yes")
	}
}

// Output:
// yes
