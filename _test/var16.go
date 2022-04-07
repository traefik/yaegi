package main

func getArray() ([]int, error) { println("getArray"); return []int{1, 2}, nil }

func getNum() (int, error) { println("getNum"); return 3, nil }

func main() {
	if a, err := getNum(); err != nil {
		println("#1", a)
	} else if a, err := getArray(); err != nil {
		println("#2", a)
	}
	println("#3")
}

// Output:
// getNum
// getArray
// #3
