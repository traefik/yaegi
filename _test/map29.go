package main

import (
	"fmt"
	"time"
)

type Item struct {
	Object any
	Expiry time.Duration
}

func main() {
	items := map[string]Item{}

	items["test"] = Item{
		Object: "test",
		Expiry: time.Second,
	}

	item := items["test"]
	fmt.Println(item)
}

// Output:
// {test 1s}
