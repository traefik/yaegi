package main

func f() {
	println("in goroutine f")
}

func main() {
	go f()
	//sleep(100)
	println("in main")
}
