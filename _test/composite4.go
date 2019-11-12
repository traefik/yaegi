package main

func main() {
	var err error

	_, ok := err.(interface{ IsSet() bool })
	println(ok)
}

// Output:
// false
