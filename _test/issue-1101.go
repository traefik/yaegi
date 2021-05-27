package main

import (
	"fmt"
	"net/http"
)

func main() {
	method := "POST"
	switch method {
	case http.MethodPost:
		fmt.Println("It's a post!")
	}
}

// Output:
// It's a post!
