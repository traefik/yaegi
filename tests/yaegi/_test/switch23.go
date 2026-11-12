package main

func getType() string { return "T1" }

func main() {
	switch getType() {
	case "T1":
		println("T1")
	default:
		println("default")
	}
}

// Output:
// T1
