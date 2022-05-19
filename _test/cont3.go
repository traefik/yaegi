package main

func main() {
	n := 2
	m := 2
	foo := true
OuterLoop:
	println("boo")
	for i := 0; i < n; i++ {
		println("I: ", i)
		for j := 0; j < m; j++ {
			switch foo {
			case true:
				println(foo)
				continue OuterLoop
			case false:
				println(foo)
			}
		}
	}
}

// Error:
// 15:5: invalid continue label OuterLoop
