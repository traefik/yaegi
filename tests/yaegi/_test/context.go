package main

import "context"

func get(ctx context.Context, k string) string {
	var r string
	if v := ctx.Value(k); v != nil {
		r = v.(string)
	}
	return r
}

func main() {
	ctx := context.WithValue(context.Background(), "hello", "world")
	println(get(ctx, "hello"))
}

// Output:
// world
