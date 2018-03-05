package main

type T2 struct {
        h int
}

type T struct { 
        f int
        g int
        T2
}       

func f(i int) int { return i * i }

func main() {
	a := T{ 5, 7, T2{ h: f(8) } }
        println(a.f, a.g, a.T2.h)
}
