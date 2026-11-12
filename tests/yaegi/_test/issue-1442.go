package main

import (
	"context"
)

func main() {
	ctx, _ := context.WithCancel(context.Background())
	ch := make(chan string, 20)
	defer close(ch)

	go func(ctx context.Context, ch <-chan string) {
		for {
			select {
			case <-ctx.Done():
				return
			case tmp := <-ch:
				_ = tmp
			}
		}
	}(ctx, ch)

	for _, i := range "abcdef" {
		for _, j := range "0123456789" {
			// i, j := "a", "0"
			for _, k := range "ABCDEF" {
				select {
				case <-ctx.Done():
					return
				default:
					tmp := string(i) + string(j) + string(k)
					ch <- tmp
				}
			}
		}
	}
	return
}

// Output:
//
