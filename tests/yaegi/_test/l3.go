package main

//func myprint(i int) { println(i) }

func main() {
	for a := 0; a < 20000000; a++ {
		if a&0x8ffff == 0x80000 {
			println(a)
			//myprint(a)
		}
	}
}
