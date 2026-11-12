package main

func adder() func(int) int {
	sum := 0
	return func(x int) int {
		sum = sum + x
		return sum
	}
}

func main() {
	pos, neg := adder(), adder()
	for i := 0; i < 10; i++ {
		println(pos(i), neg(-2*i))
	}
}

// Output:
// 0 0
// 1 -2
// 3 -6
// 6 -12
// 10 -20
// 15 -30
// 21 -42
// 28 -56
// 36 -72
// 45 -90
