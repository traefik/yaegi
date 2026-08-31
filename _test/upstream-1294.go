package main

type RedisError string

func (e RedisError) Error() string { return string(e) }

const Nil = RedisError("redis: nil")

func main() {
	println(Nil == Nil)
}

// Output:
// true
