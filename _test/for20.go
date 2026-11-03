package main

func main() {
	n := 0
	for i := 0; i < 10; i++ {
		n++
		i++
	}
	println(n)

	m := 0
	for i := 0; i < 10; i++ {
		m++
		i += 3
	}
	println(m)

	c := 0
	for i := 0; i < 9; i++ {
		c++
		if i%2 == 0 {
			i++
			continue
		}
		c += 100
	}
	println(c)
}

// Output:
// 5
// 3
// 5
