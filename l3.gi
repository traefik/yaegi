#!/usr/bin/env gi
package main

func main() {
//for a := 0; a < 10000; a++ {
for a := 0; a < 20000000; a++ {
	//if a & 0x8ff == 0x800 {
	if a & 0x8ffff == 0x80000 {
		//println(a)
		myprint(a)
	}
}
}

func myprint(i int) { println(i) }

