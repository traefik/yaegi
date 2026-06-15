package main

type I interface {
	Get() any
}

type T struct{}

func (T) Get() any {
	return nil
}

func main() {
	var i I = T{}
	var ei any

	println(i != ei)
}

// Output:
// true
