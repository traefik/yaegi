package main

func main() {
	i := 0
	for {
		if i > 10 {
			break
		}
		if i < 5 {
			i++
			continue
		}
		println(i)
		i++
	}
}
