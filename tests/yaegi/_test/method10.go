package main

const BlockSize = 8

type Cipher struct{}

func (c *Cipher) BlockSize() int { return BlockSize }

func main() {
	println(BlockSize)
	s := Cipher{}
	println(s.BlockSize())
}

// Output:
// 8
// 8
