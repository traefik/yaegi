package main

func genInt() (int, error) { return 3, nil }

func getInt() (value int) {
	value, err := genInt()
	if err != nil {
		panic(err)
	}
	return
}

func main() {
	println(getInt())
}

// Output:
// 3
