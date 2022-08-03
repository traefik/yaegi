package main

type Number interface {
	int | int64 | ~float64
}

func Sum[T Number](numbers []T) T {
	var total T
	for _, x := range numbers {
		total += x
	}
	return total
}

func main() {
	xs := []int{3, 5, 10}
	total := Sum(xs)
	println(total)
}

// Output:
// 18
