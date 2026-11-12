package main

import "time"

var UnixTime func(int64, int64) time.Time

func main() {
	UnixTime = time.Unix
	println(UnixTime(1e9, 0).In(time.UTC).Minute())
}

// Output:
// 46
