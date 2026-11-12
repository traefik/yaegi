package main

// context.WithValue with interpreted keys and values (run in the default
// and -aot modes by TestTestdata).

import (
	"context"
	"fmt"
	"net/http/httptest"
	"time"
)

type userKey struct{}
type roleKey struct{} // same layout as userKey: must not collide

type ctxKey string

type Session struct{ User string }

func (s *Session) Name() string { return s.User }

func main() {
	ctx := context.WithValue(context.Background(), userKey{}, &Session{"bob"})
	ctx = context.WithValue(ctx, roleKey{}, "admin")
	ctx = context.WithValue(ctx, ctxKey("trace"), 42)
	ctx = context.WithValue(ctx, "plain", 1.5)

	s, ok := ctx.Value(userKey{}).(*Session)
	fmt.Println(ok, s.Name(), ctx.Value(roleKey{}), ctx.Value(ctxKey("trace")), ctx.Value("plain"))
	fmt.Println(ctx.Value(ctxKey("missing")) == nil, ctx.Value("trace") == nil)

	// Through native contexts derived from the interpreted one.
	cctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	fmt.Println(cctx.Value(userKey{}).(*Session).User, cctx.Value(ctxKey("trace")))

	// Through an http.Request, as middlewares do.
	r := httptest.NewRequest("GET", "/", nil).WithContext(cctx)
	if s, ok := r.Context().Value(userKey{}).(*Session); ok {
		fmt.Println("request:", s.Name(), r.Context().Value(roleKey{}))
	}

	// Shadowing: the innermost value wins.
	inner := context.WithValue(r.Context(), userKey{}, &Session{"alice"})
	fmt.Println(inner.Value(userKey{}).(*Session).Name(), cctx.Value(userKey{}).(*Session).Name())
}
