package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	http.DefaultClient.Timeout = time.Second * 10
	fmt.Println(http.DefaultClient)
	http.DefaultClient = &http.Client{}
	fmt.Println(http.DefaultClient)
}

// Output:
// &{<nil> <nil> <nil> 10s}
// &{<nil> <nil> <nil> 0s}
