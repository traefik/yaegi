package main

func test() error { return nil }

func main() {
	if err := test(); nil == err {
		println("err is nil")
	}
}

// Output:
// err is nil
