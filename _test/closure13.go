package main

import (
	"fmt"
	"strconv"
)

type monkey struct {
	test func() int
}

func main() {
	input := []string{"1", "2", "3"}

	var monkeys []*monkey

	for _, v := range input {
		kong := monkey{}
		divisor, err := strconv.Atoi(v)
		if err != nil {
			panic(err)
		}
		fmt.Print(divisor, " ")
		kong.test = func() int {
			return divisor
		}
		monkeys = append(monkeys, &kong)
	}

	for _, mk := range monkeys {
		fmt.Print(mk.test(), " ")
	}
}

// Output:
// 1 2 3 1 2 3
