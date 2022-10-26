package main

type Number int32

type Number1 = Number

type Number2 = Number1

func (n Number2) IsValid() bool { return true }

func main() {
	a := Number(5)
	println(a.IsValid())
}

// Output:
// true
