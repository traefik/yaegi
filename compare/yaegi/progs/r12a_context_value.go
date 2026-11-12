package main

import (
	"context"
	"fmt"
)

type key struct{}

type Session struct{ User string }

func (s *Session) Name() string { return s.User }

func main() {
	ctx := context.WithValue(context.Background(), key{}, &Session{"bob"})
	s, ok := ctx.Value(key{}).(*Session)
	fmt.Println(ok, s.Name())
	ctx2 := context.WithValue(ctx, "str", 42)
	fmt.Println(ctx2.Value("str"), ctx2.Value(key{}).(*Session).User)
}
