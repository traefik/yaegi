package main

import "fmt"

type monkey struct {
	test func() int
}

func getk(k int) (int, error) { return k, nil }

func main() {
	input := []string{"1", "2", "3"}

	var monkeys []*monkey

	for k := range input {
		kong := monkey{}
		divisor, _ := getk(k)
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
// 0 1 2 0 1 2
