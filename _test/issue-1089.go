package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println(`"` + time.RFC3339Nano + `"`)
}

// Output:
// "2006-01-02T15:04:05.999999999Z07:00"
