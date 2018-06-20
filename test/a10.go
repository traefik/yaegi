package main

type Sample struct {
	Name string
}

var samples = []Sample{}

func f(i int) {
	println(samples[i].Name)
}

func main() {
	samples = append(samples, Sample{Name: "test"})
	f(0)
}

// Output:
// test
