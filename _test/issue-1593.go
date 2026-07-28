package main

import (
	"context"
)

func main() {
	checkImpl(context)
}

func checkImpl(ctx context.Context) {
	// intentionally left blank
}

// Error:
// 8:12: use of package context not in selector
