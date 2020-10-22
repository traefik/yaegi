package main

import "fmt"

func main() {
    a := [2][2]int{{0, 1}, {2, 3}}
    fmt.Println(a[0][0:])
}

// Output:
// [0 1]
