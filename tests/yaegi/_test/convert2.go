package main

import "bufio"

func fakeSplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	return 7, nil, nil
}

func main() {
	splitfunc := bufio.SplitFunc(fakeSplitFunc)
	n, _, err := splitfunc(nil, true)
	if err != nil {
		panic(err)
	}
	println(n)
}

// Output:
// 7
