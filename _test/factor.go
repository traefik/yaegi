package main

import (
	"fmt"
	"math/big"
)

func main() {
	// 157 bit n = pq with p ~= 78 bits
	n := big.NewInt(0)
	n.SetString("273966616513101251352941655302036077733021013991", 10)

	i := big.NewInt(0)
	// Set i to be p - 10e6
	i.SetString("496968652506233112158689", 10)

	// Move temp big int out here so no possible GC thrashing
	temp := big.NewInt(0)
	// Avoid creating the new bigint each time
	two := big.NewInt(2)
	for {
		// Check if the odd number is a divisor of n
		temp.Mod(n, i)
		if temp.Sign() == 0 {
			fmt.Println(i)
			break
		}

		i.Add(i, two)
	}
}
