package main

import "fmt"

type T1 struct {
	P []*T
}

type T2 struct {
	P2 *T
}

type T struct {
	*T1
	S1 *T
}

func main() {
	fmt.Println(T2{})
}

// Output:
// {<nil>}
