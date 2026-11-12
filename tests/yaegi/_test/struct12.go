package main

import "fmt"

type S1 struct {
	Name string
}

type S2 struct {
	*S1
}

func main() {
	fmt.Println(S2{})
}

// Output:
// {<nil>}
