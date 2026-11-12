package main

func main() {
	s := 0
	for i := 0; i < 10000000; i++ {
		s += i
	}
	println(s)
}
