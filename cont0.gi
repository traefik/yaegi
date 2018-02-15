package main

func main() {
	i := 0
	for {
		if i > 10 {
			break
		}
		i++
		if i < 5 {
			continue
		}
		println(i)
	}
}
