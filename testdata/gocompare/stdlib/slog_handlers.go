// log/slog with program types: TextHandler and JSONHandler writing to a
// strings.Builder with ReplaceAttr removing the time, LogValuer on program
// types, groups, With, levels, Stringer and error values, a custom Handler.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type User struct {
	ID    int
	Name  string
	Token string
}

func (u User) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("id", u.ID), slog.String("name", u.Name))
}

type Secret string

func (Secret) LogValue() slog.Value { return slog.StringValue("REDACTED") }

type Lazy struct{ n int }

func (l *Lazy) LogValue() slog.Value { l.n++; return slog.IntValue(l.n * 10) }

type Point struct{ X, Y int }

func (p Point) String() string { return fmt.Sprintf("(%d,%d)", p.X, p.Y) }

type MyErr struct{ Code int }

func (e *MyErr) Error() string { return fmt.Sprintf("my error %d", e.Code) }

// prefixHandler is a program Handler that records what it receives.
type prefixHandler struct {
	sb     *strings.Builder
	attrs  []slog.Attr
	groups []string
}

func (h *prefixHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelInfo }
func (h *prefixHandler) Handle(_ context.Context, r slog.Record) error {
	fmt.Fprintf(h.sb, "[%s] %s groups=%v", r.Level, r.Message, h.groups)
	for _, a := range h.attrs {
		fmt.Fprintf(h.sb, " pre:%s=%v", a.Key, a.Value)
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(h.sb, " %s=%v(%s)", a.Key, a.Value.Resolve(), a.Value.Resolve().Kind())
		return true
	})
	fmt.Fprintf(h.sb, " n=%d\n", r.NumAttrs())
	return nil
}
func (h *prefixHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &prefixHandler{h.sb, append(append([]slog.Attr{}, h.attrs...), as...), h.groups}
}
func (h *prefixHandler) WithGroup(g string) slog.Handler {
	return &prefixHandler{h.sb, h.attrs, append(append([]string{}, h.groups...), g)}
}

func noTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		return slog.Attr{}
	}
	if a.Key == "token" {
		return slog.String("token", "***")
	}
	if a.Value.Kind() == slog.KindDuration {
		return slog.String(a.Key, a.Value.Duration().String())
	}
	return a
}

func logAll(l *slog.Logger) {
	ctx := context.Background()
	u := User{1, "ann", "tok"}
	lazy := &Lazy{}
	l.Info("hello", "user", u, "secret", Secret("pw"), "lazy", lazy, "pt", Point{1, 2})
	l.Debug("hidden debug")
	l.Warn("warn", "err", &MyErr{7}, "wrapped", fmt.Errorf("ctx: %w", &MyErr{8}), "nilerr", error(nil))
	l.Error("error", slog.Group("req", "method", "GET", slog.Int("status", 500)), "dur", 1500*time.Millisecond)
	l.Log(ctx, slog.LevelWarn+2, "custom level", "f", 1.5, "b", true, "u", uint64(1<<63), "i", int64(-5))
	l.LogAttrs(ctx, slog.LevelInfo, "attrs", slog.Any("slice", []int{1, 2}), slog.Any("map", map[string]int{"b": 2, "a": 1}), slog.Time("t", time.Date(2024, 1, 2, 3, 4, 5, 6000, time.UTC)))
	l.Info("odd", "lonely")
	l.Info("badkey", 42, "v")
	l.Info("quote me", "s", "has space", "e", "", "q", `a"b`, "nl", "x\ny")
	l2 := l.With("svc", "api", "user", u).WithGroup("g")
	l2.Info("with group", "a", 1, slog.Group("inner", "b", 2), slog.Group("empty"))
	l2.WithGroup("h").Info("nested", "c", 3, "token", "abc")
	l.Info("lazy again", "lazy", lazy)
}

func main() {
	var sb strings.Builder
	opts := &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: noTime}
	logAll(slog.New(slog.NewTextHandler(&sb, opts)))
	fmt.Print(sb.String())
	sb.Reset()
	logAll(slog.New(slog.NewJSONHandler(&sb, opts)))
	fmt.Print(sb.String())
	sb.Reset()
	logAll(slog.New(&prefixHandler{sb: &sb}))
	fmt.Print(sb.String())

	// Levels and LevelVar.
	var lv slog.LevelVar
	lv.Set(slog.LevelWarn)
	sb.Reset()
	l := slog.New(slog.NewTextHandler(&sb, &slog.HandlerOptions{Level: &lv, ReplaceAttr: noTime, AddSource: false}))
	l.Info("no")
	l.Warn("yes")
	lv.Set(slog.LevelDebug)
	l.Debug("now yes")
	fmt.Print(sb.String())
	for _, lvl := range []slog.Level{slog.LevelDebug - 1, slog.LevelDebug, slog.LevelInfo + 1, slog.LevelWarn, slog.LevelError + 3} {
		b, _ := lvl.MarshalText()
		fmt.Print(lvl, " ", string(b), "; ")
	}
	fmt.Println()
	var parsed slog.Level
	fmt.Println(parsed.UnmarshalText([]byte("warn+1")), parsed, parsed.UnmarshalText([]byte("nope")))
	// Values.
	for _, v := range []slog.Value{slog.AnyValue(3), slog.AnyValue(uint8(3)), slog.AnyValue(Point{}), slog.AnyValue(User{ID: 2}), slog.AnyValue(nil), slog.AnyValue(time.Minute), slog.AnyValue(errors.New("e"))} {
		fmt.Println(v.Kind(), v, v.Resolve().Kind())
	}
	a := slog.Group("g", slog.Int("x", 1))
	fmt.Println(a, a.Equal(slog.Group("g", slog.Int("x", 1))), a.Equal(slog.Group("g", slog.Int("x", 2))))
}
