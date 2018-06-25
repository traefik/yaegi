package main

func main() {
	for a := 0; a < 20000; a++ {
		if (a & 0x8ff) == 0x800 {
			println(a)
		}
	}
}
