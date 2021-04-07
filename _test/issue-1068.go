package main

type I interface {
	Hello()
}

type T struct{}

func (t T) Hello() { println("hello") }

type I2 I

func main() {
	var i I2 = T{}
	i.Hello()
}

// Output:
// hello
