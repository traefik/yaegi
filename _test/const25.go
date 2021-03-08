package main

const (
	FGBlack Attribute = iota + 30
)

type Attribute int

func main() {
	println(FGBlack)
}
