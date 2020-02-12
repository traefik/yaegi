package main

const (
	maxNonStarters = 30
	maxBufferSize  = maxNonStarters + 2
)

type reorderBuffer struct {
	rune [maxBufferSize]Properties
}

type Properties struct {
	pos  uint8
	size uint8
}

func main() {
	println(len(reorderBuffer{}.rune))
}

// Output:
// 32
