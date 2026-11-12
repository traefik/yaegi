package main

import "fmt"

func main() {
	count := 0
	i := 0
	const (
		stateBefore = 0
		statePair   = 1
	)
	state := stateBefore
	for i < 4 {
		i++
		switch state {
		case stateBefore:
			state = statePair
			if i == 2 {
				goto nextPair
			}
			count += 10
		case statePair:
			if i == 3 {
				goto nextPair
			}
			count += 1
		}
	nextPair:
		state = stateBefore
	}
	fmt.Println("count:", count)
}

// Output:
// count: 30
