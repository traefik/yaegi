package main

import (
	"fmt"
)

type Config struct {
	A string
}

var conf *Config = &Config{}

func SetConfig() func(*Config) {
	return func(cf *Config) {
		conf = cf
	}
}

func main() {
	conf := &Config{
		A: "foo",
	}

	fmt.Println(conf.A)
}

// Output:
// foo
