package main

type I interface {
	Hello()
}

type T struct {
	Name  string
	Child []*T
}

func (t *T) Hello() { println("Hello", t.Name) }

func main() {
	var i I = new(T)
	i.Hello()
}

// Output:
// Hello
