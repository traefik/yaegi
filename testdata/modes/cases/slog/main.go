// log/slog: text and JSON handlers with interpreted types (LogValuer,
// Stringer, MarshalJSON, error, structs), groups and a custom handler.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

type User struct {
	ID   int
	Name string
}

func (u User) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("id", u.ID), slog.String("name", u.Name))
}

type Token string

func (Token) LogValue() slog.Value { return slog.StringValue("REDACTED") }

type Level int

func (l Level) String() string { return fmt.Sprintf("L%d", int(l)) }

type Raw struct{ A int }

func (r Raw) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`{"raw":%d}`, r.A)), nil }

type MyErr struct{ Code int }

func (e MyErr) Error() string { return fmt.Sprintf("code %d", e.Code) }

// prefix is an interpreted slog.Handler.
type prefix struct {
	slog.Handler
	p string
}

func (h prefix) Handle(ctx context.Context, r slog.Record) error {
	r.Message = h.p + r.Message
	return h.Handler.Handle(ctx, r)
}

func main() {
	noTime := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}
	opts := &slog.HandlerOptions{ReplaceAttr: noTime}
	for _, h := range []slog.Handler{slog.NewTextHandler(os.Stdout, opts), slog.NewJSONHandler(os.Stdout, opts)} {
		l := slog.New(h)
		l.Info("login", "user", User{1, "ann"}, "token", Token("secret"), "level", Level(3), "raw", Raw{4}, "err", MyErr{5}, "plain", Raw{6}, "ptr", &User{2, "bob"})
		l.With("svc", "api").WithGroup("req").Warn("slow", slog.Any("u", User{3, "c"}), slog.Group("g", "lv", Level(1)))
		l.Error("failed", "err", errors.Join(MyErr{7}, errors.New("io")))
		slog.New(prefix{h, "[p] "}).Info("wrapped", "n", 1)
	}
}
