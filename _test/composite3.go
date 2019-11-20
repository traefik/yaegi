package main

func main() {
	var err error
	var ok bool

	_, ok = err.(interface{ IsSet() bool })
	println(ok)
}

// Output:
// false
