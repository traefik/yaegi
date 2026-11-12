package main

import (
	"context"
	"fmt"
)

type ctxKey string

type Session struct{ User string }

func main() {
	ctx := context.WithValue(context.Background(), ctxKey("s"), &Session{"bob"})
	v := ctx.Value(ctxKey("s"))
	fmt.Println(v != nil)
	s, ok := v.(*Session)
	fmt.Println(ok, s.User)
	ctx = context.WithValue(ctx, ctxKey("t"), "str")
	fmt.Println(ctx.Value(ctxKey("t")))
}
