package main

func main() {
	var err error
	//var ok bool

	_, ok := err.(interface{ IsSet() bool })
	println(ok)
	//_, ok = err.(interface{ Error() string })
	//println(ok)
}

// Output:
// false
