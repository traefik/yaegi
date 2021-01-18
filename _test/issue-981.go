package main

import "fmt"

func main() {
	dp := make(map[int]int)
	dp[0] = 1
	for i := 1; i < 10; i++ {
		dp[i] = dp[i-1] + dp[i-2]
	}
	fmt.Printf("%v\n", dp)
}

// Output:
// map[0:1 1:1 2:2 3:3 4:5 5:8 6:13 7:21 8:34 9:55]
