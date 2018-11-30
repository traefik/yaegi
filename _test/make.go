package main

import (
	"fmt"
	"net/http"
)

func main() {
	h := make(http.Header)
	fmt.Println("h:", h)
}

// Output:
// h: map[]
