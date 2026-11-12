// AOT test program: log/slog with interpreted values.
//
//aot:lib log/slog
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Level int

func (l Level) String() string { return "L" + strconv.Itoa(int(l)) }

// User implements slog.LogValuer.
type User struct {
	ID    int
	Name  string
	token string
}

func (u User) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("id", u.ID), slog.String("name", u.Name))
}

type Point struct{ X, Y int }

type Secret string

func (Secret) LogValue() slog.Value { return slog.StringValue("REDACTED") }

type Temp float64

func (t Temp) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"%.1fC"`, float64(t))), nil }

// countHandler is an interpreted slog.Handler wrapping a native one.
type countHandler struct {
	inner slog.Handler
	n     *int
}

func (h countHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.inner.Enabled(ctx, l) }
func (h countHandler) Handle(ctx context.Context, r slog.Record) error {
	*h.n++
	r.AddAttrs(slog.Int("seq", *h.n))
	return h.inner.Handle(ctx, r)
}
func (h countHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return countHandler{h.inner.WithAttrs(as), h.n}
}
func (h countHandler) WithGroup(name string) slog.Handler {
	return countHandler{h.inner.WithGroup(name), h.n}
}

func noTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		return slog.Attr{}
	}
	return a
}

func main() {
	if benchMain(benchOp) {
		return
	}
	opts := &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: noTime}
	for _, h := range []slog.Handler{slog.NewTextHandler(os.Stdout, opts), slog.NewJSONHandler(os.Stdout, opts)} {
		l := slog.New(h)
		l.Info("hello", "user", User{1, "ann", "tok"}, "lvl", Level(2), "pt", Point{1, 2}, "pp", &Point{3, 4})
		l.Debug("secret", "pw", Secret("x"), "err", errors.New("boom"), "t", Temp(21.5), "d", 1500*time.Millisecond)
		l.With("svc", "api").WithGroup("req").Warn("grouped", slog.Group("g", "a", 1, "b", Level(0)), "m", map[string]int{"k": 1})
		l.Log(context.Background(), slog.LevelError, "err", "list", []any{Level(1), "s"}, slog.Any("any", Point{}))
	}
	n := 0
	var sb strings.Builder
	l := slog.New(countHandler{slog.NewTextHandler(&sb, opts), &n})
	l.Info("one", "u", User{2, "bo", ""})
	l.With("k", Level(1)).Error("two")
	fmt.Print(sb.String())
	fmt.Println(n)
	v := slog.AnyValue(User{3, "cy", ""})
	fmt.Println(v.Kind(), v.Resolve().Kind(), slog.AnyValue(Level(1)).Kind(), slog.AnyValue(7).Kind())
	var lv slog.Level
	fmt.Println(lv.UnmarshalText([]byte("WARN+2")), lv, slog.LevelInfo.String())
}

var benchLogger = slog.New(slog.NewJSONHandler(discard{}, nil))

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// benchOp: one JSON log line with a LogValuer and a Stringer.
func benchOp() {
	benchLogger.Info("req", "user", User{1, "ann", ""}, "lvl", Level(2), "n", 3)
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
