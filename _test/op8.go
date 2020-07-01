package main

type I interface {
	Get() interface{}
}

type T struct{}

func (T) Get() interface{} {
	return nil
}

func main() {
	var i I = T{}
	var ei interface{}

	println(i != ei)
}

// Output:
// true
