package main

import (
	"fmt"
	"strconv"
)

var optionsG map[string]string

var roundG int = 30

func strToInt(s string, defaultValue int) int {
	n, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return defaultValue
	}
	return int(n)
}

func main() {
	optionsG := map[string]string{"round": "12", "b": "one"}
	roundG = strToInt(optionsG["round"], 50)
	fmt.Println(roundG)
	fmt.Println(optionsG)
}

// Output:
// 12
// map[b:one round:12]
