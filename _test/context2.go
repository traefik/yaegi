package main

import "context"

func get(ctx context.Context, k string) string {
	var r string
	var ok bool
	if v := ctx.Value(k); v != nil {
		r, ok = v.(string)
		println(ok)
	}
	return r
}

func main() {
	ctx := context.WithValue(context.Background(), "hello", "world")
	println(get(ctx, "hello"))
}

// Output:
// true
// world
