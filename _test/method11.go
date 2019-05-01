package main

func BlockSize() string { return "func" }

type Cipher struct{}

func (c *Cipher) BlockSize() string { return "method" }

func main() {
	println(BlockSize())
	s := Cipher{}
	println(s.BlockSize())
}

// Output:
// func
// method
