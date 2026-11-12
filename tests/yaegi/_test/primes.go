package main

func Primes(n int) int {
	var xs []int
	for i := 2; len(xs) < n; i++ {
		ok := true
		for _, x := range xs {
			if i%x == 0 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		xs = append(xs, i)
	}
	return xs[n-1]
}

func main() {
	println(Primes(3))
}

// Output:
// 5
