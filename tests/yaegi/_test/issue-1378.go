package main

import (
	"fmt"
	"time"
)

func main() {
	t, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
	if err != nil {
		panic(err)
	}
	fn := func() error {
		_, err := t.GobEncode()
		return err
	}
	fmt.Println(fn())
}

// Output:
// <nil>
