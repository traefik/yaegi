package main

func f() int { println("in f"); return 1 }

func main() {
	for i := f(); ; {
		println("in loop")
		if i > 0 {
			break
		}
	}
}

// Output:
// in f
// in loop
