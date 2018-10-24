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
		println(pos(i), neg(0-2*i))

	}
}
