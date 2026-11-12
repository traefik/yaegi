package main

/*
static int add(int a, int b) { return a + b; }
*/
import "C"

import "fmt"

func main() {
	fmt.Println(int(C.add(2, 3)))
}
