package main

import "fmt"

type Sample struct {
	Name string
}

var samples = []Sample{}

func main() {
	samples = append(samples, Sample{Name: "test"})
	fmt.Println(samples)
}
