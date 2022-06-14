package main

type Number int32

func (n Number) IsValid() bool { return true }

type Number1 = Number

type Number2 = Number1

func main() {
	a := Number2(5)
	println(a.IsValid())
}

// Output: true
