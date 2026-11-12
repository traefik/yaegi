package main

import (
	"fmt"
	"net/http/httptest"
)

func main() {
	recorder := httptest.NewRecorder()
	recorder.Header().Add("Foo", "Bar")

	for key, value := range recorder.Header() {
		fmt.Println(key, value)
	}
}

// Output:
// Foo [Bar]
