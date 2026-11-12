package main

import (
	"fmt"
	"time"
)

var twentyFourHours = time.Duration(24 * time.Hour)

func main() {
	fmt.Println(twentyFourHours.Hours())
}

// Output:
// 24
