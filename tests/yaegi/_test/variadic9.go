package main

import "fmt"

func Sprintf(format string, a ...interface{}) string {
	return fmt.Sprintf(format, a...)
}

func main() {
	fmt.Println(Sprintf("Hello %s", "World!"))
}

// Output:
// Hello World!
