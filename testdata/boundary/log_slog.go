// Native boundary of log/slog: program types given to slog values, loggers
// and the text and JSON handlers (docs/BOUNDARY.md).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// opts logs every level and removes the time of the record.
var opts = &slog.HandlerOptions{
	Level: slog.LevelDebug,
	ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	},
}

// both runs f with a text logger and with a JSON logger (also set as the
// default logger), and prints what they wrote.
func both(f func(l *slog.Logger, h slog.Handler)) {
	for _, js := range []bool{false, true} {
		var sb strings.Builder
		var h slog.Handler = slog.NewTextHandler(&sb, opts)
		name := "text"
		if js {
			h = slog.NewJSONHandler(&sb, opts)
			name = "json"
		}
		l := slog.New(h)
		slog.SetDefault(l)
		f(l, h)
		for _, line := range strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n") {
			out("%s %s", name, line)
		}
	}
}

// try calls f and prints its panic.
func try(name string, f func() any) {
	defer func() {
		if r := recover(); r != nil {
			out("%s: panic %T: %v", name, r, r)
		}
	}()
	out("%s: %v", name, f())
}

var ctx = context.Background()

// Named basic types: slog must not take the path of the basic type.
type (
	B     bool
	F32   float32
	F64   float64
	I     int
	I8    int8
	I16   int16
	I32   int32
	I64   int64
	S     string
	U     uint
	U8    uint8
	U16   uint16
	U32   uint32
	U64   uint64
	Uptr  uintptr
	Bs    []byte
	Bs8   []U8
	attrs []slog.Attr
	kind  slog.Kind
	value slog.Value
	attr  slog.Attr
	dur   time.Duration
)

// lvv is a LogValuer (value receiver).
type lvv struct{ n int }

func (v lvv) LogValue() slog.Value { return slog.IntValue(v.n * 10) }

// lvp is a LogValuer (pointer receiver) that gives a group with a LogValuer.
type lvp struct{ name string }

func (p *lvp) LogValue() slog.Value {
	if p == nil {
		return slog.StringValue("lvp<nil>")
	}
	return slog.GroupValue(slog.String("name", p.name), slog.Any("inner", lvv{7}))
}

// lvg is a generic LogValuer.
type lvg[T any] struct{ v T }

func (g lvg[T]) LogValue() slog.Value { return slog.AnyValue(g.v) }

// lvrec gives another LogValuer until depth is 0.
type lvrec struct{ depth int }

func (r lvrec) LogValue() slog.Value {
	if r.depth == 0 {
		return slog.StringValue("bottom")
	}
	return slog.AnyValue(lvrec{r.depth - 1})
}

// lvloop never ends: slog stops it with an error.
type lvloop struct{}

func (lvloop) LogValue() slog.Value { return slog.AnyValue(lvloop{}) }

// lvpanic panics in LogValue.
type lvpanic struct{}

func (lvpanic) LogValue() slog.Value { panic("lvpanic") }

// secret is a named string with LogValue.
type secret string

func (secret) LogValue() slog.Value { return slog.StringValue("REDACTED") }

// tm has MarshalText (value receiver).
type tm struct{ n int }

func (t tm) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("tm<%d>", t.n)), nil }

// tmp has MarshalText (pointer receiver).
type tmp struct{ n int }

func (t *tmp) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("tmp<%d>", t.n)), nil }

// tmErr fails in MarshalText.
type tmErr struct{}

func (tmErr) MarshalText() ([]byte, error) { return nil, errors.New("tmErr failed") }

// tmPanic panics in MarshalText.
type tmPanic struct{}

func (tmPanic) MarshalText() ([]byte, error) { panic("tmPanic") }

// jm has MarshalJSON (value receiver).
type jm struct{ n int }

func (j jm) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`{"jm":%d}`, j.n)), nil }

// jmp has MarshalJSON (pointer receiver).
type jmp struct{ n int }

func (j *jmp) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`["jmp",%d]`, j.n)), nil }

// errJM is an error and a json.Marshaler: JSON uses MarshalJSON.
type errJM struct{}

func (errJM) Error() string                { return "errJM.Error" }
func (errJM) MarshalJSON() ([]byte, error) { return []byte(`"errJM.JSON"`), nil }

// ev is an error (value receiver).
type ev struct{ msg string }

func (e ev) Error() string { return "ev:" + e.msg }

// ep is an error (pointer receiver).
type ep struct{ msg string }

func (e *ep) Error() string { return "ep:" + e.msg }

// ta has only AppendText (encoding.TextAppender).
type ta struct{ n int }

func (t ta) AppendText(b []byte) ([]byte, error) { return fmt.Appendf(b, "ta<%d>", t.n), nil }

// tmta has MarshalText and AppendText.
type tmta struct{}

func (tmta) MarshalText() ([]byte, error)        { return []byte("tmta.Marshal"), nil }
func (tmta) AppendText(b []byte) ([]byte, error) { return append(b, "tmta.Append"...), nil }

// sv has a String method.
type sv struct{ n int }

func (s sv) String() string { return fmt.Sprintf("sv<%d>", s.n) }

// plain has no methods.
type plain struct {
	A int
	b string
}

// withSlice is not comparable.
type withSlice struct{ L []int }

func main() {
	fixed := time.Date(2024, 2, 3, 4, 5, 6, 7000000, time.UTC)
	named := []any{B(true), F32(1.5), F64(2.5), I(-1), I8(-8), I16(16), I32(32), I64(64), S("s"), U(1), U8(8), U16(16),
		U32(32), U64(64), Uptr(9), Bs("bytes"), Bs8{104, 105}, attrs{slog.Int("a", 1)}, kind(slog.KindInt64),
		value(slog.IntValue(3)), attr(slog.String("k", "v")), dur(time.Second)}

	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*Logger).Debug,(*Logger).DebugContext,(*Logger).Error,(*Logger).ErrorContext,(*Logger).Info,(*Logger).InfoContext,(*Logger).Log,(*Logger).Warn,(*Logger).WarnContext,(*Logger).With,(*Record).Add,(*TextHandler).Handle,(*TextHandler).WithAttrs,Any,AnyValue,Debug,DebugContext,Error,ErrorContext,Group,Info,InfoContext,Log,Value.Resolve,Warn,WarnContext,With assert x.(T), T unnamed
	probe("Unnamed", func() {
		for i, x := range named {
			v := slog.AnyValue(x)
			out("%d %T AnyValue kind=%v %v | Any kind=%v | Resolve kind=%v", i, x, v.Kind(), v, slog.Any("k", x).Value.Kind(), v.Resolve().Kind())
		}
		both(func(l *slog.Logger, h slog.Handler) {
			l.Info("named", "b", B(true), "i", I(1), "s", S("x"), "f", F64(0.5), "bs", Bs("y"), "u8", U8(2))
			l.Debug("d", "bs8", Bs8{65}, "attrs", attrs{slog.Int("a", 1)}, "kind", kind(slog.KindBool))
			l.DebugContext(ctx, "dc", "value", value(slog.IntValue(3)), "attr", attr(slog.String("k", "v")))
			l.Warn("w", "dur", dur(time.Millisecond), "u", U(3))
			l.WarnContext(ctx, "wc", "f32", F32(0.25))
			l.Error("e", "i8", I8(-1))
			l.ErrorContext(ctx, "ec", "u64", U64(1<<63))
			l.InfoContext(ctx, "ic", "i64", I64(-64))
			l.Log(ctx, slog.LevelInfo, "log", "uptr", Uptr(5))
			// a named string or a named Attr is not a key
			l.Info("badkey", S("notkey"), 1, attr(slog.Int("x", 1)), "after", 2)
			l.With("with", I(4), S("bad")).Info("with")
			l.Info("group", slog.Group("g", "i", I(5), S("bad"), attr(slog.Int("a", 1))))
			r := slog.NewRecord(time.Time{}, slog.LevelInfo, "record", 0)
			r.Add("r", I(6), S("bad"), attrs{})
			h.Handle(ctx, r)
			h.WithAttrs([]slog.Attr{slog.Any("wa", I(7)), slog.Any("ws", S("ws"))}).Handle(ctx, r)
			slog.Info("pkg", "i", I(8))
			slog.InfoContext(ctx, "pkgc", "s", S("y"))
			slog.Debug("pkgd", "b", B(false))
			slog.DebugContext(ctx, "pkgdc", "u16", U16(1))
			slog.Warn("pkgw", "u32", U32(2))
			slog.WarnContext(ctx, "pkgwc", "i16", I16(3))
			slog.Error("pkge", "i32", I32(4))
			slog.ErrorContext(ctx, "pkgec", "f", F32(1))
			slog.Log(ctx, slog.LevelWarn, "pkglog", "bs", Bs("z"))
			slog.With("pw", S("w"), S("bad")).Info("pkgwith")
		})
	})
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*Logger).Debug,(*Logger).DebugContext,(*Logger).Error,(*Logger).ErrorContext,(*Logger).Info,(*Logger).InfoContext,(*Logger).Log,(*Logger).LogAttrs,(*Logger).Warn,(*Logger).WarnContext,(*Logger).With,(*Record).Add,(*Record).AddAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs,Any,AnyValue,Debug,DebugContext,Error,ErrorContext,Group,GroupAttrs,GroupValue,Info,InfoContext,Log,LogAttrs,Warn,WarnContext,With assert x.(log/slog.LogValuer)
	probe("LogValuer", func() {
		both(func(l *slog.Logger, h slog.Handler) {
			l.Info("lv", "v", lvv{1}, "p", &lvp{"pp"}, "nilp", (*lvp)(nil), "g", lvg[string]{"gen"}, "gi", lvg[lvv]{lvv{2}})
			l.Info("lv2", "rec", lvrec{3}, "loop", lvloop{}, "secret", secret("pw"), "iface", slog.LogValuer(lvv{3}))
			l.Debug("d", "v", lvv{4})
			l.DebugContext(ctx, "dc", "v", lvv{5})
			l.Warn("w", "p", &lvp{"w"})
			l.WarnContext(ctx, "wc", "g", lvg[int]{6})
			l.Error("e", "s", secret("x"))
			l.ErrorContext(ctx, "ec", "r", lvrec{1})
			l.InfoContext(ctx, "ic", "v", lvv{7})
			l.Log(ctx, slog.LevelInfo, "log", "v", lvv{8})
			l.LogAttrs(ctx, slog.LevelInfo, "logattrs", slog.Any("v", lvv{9}), slog.Any("p", &lvp{"la"}))
			l.With("with", lvv{10}, "wp", &lvp{"with"}).Info("with")
			l.With(slog.Group("wg", "v", lvv{11})).Info("withgroup")
			l.Info("group", slog.Group("g", "v", lvv{12}, "p", &lvp{"g"}))
			l.Info("groupattrs", slog.GroupAttrs("ga", slog.Any("v", lvv{13})))
			l.Info("groupvalue", "gv", slog.GroupValue(slog.Any("v", lvv{14}), slog.Any("g", lvg[bool]{true})))
			r := slog.NewRecord(time.Time{}, slog.LevelInfo, "record", 0)
			r.Add("v", lvv{15})
			r.AddAttrs(slog.Any("p", &lvp{"rec"}))
			h.Handle(ctx, r)
			h.WithAttrs([]slog.Attr{slog.Any("wa", lvv{16}), slog.Any("wp", &lvp{"wa"})}).Handle(ctx, r)
			slog.Info("pkg", "v", lvv{17})
			slog.InfoContext(ctx, "pkgc", "v", lvv{18})
			slog.Debug("pkgd", "v", lvv{19})
			slog.DebugContext(ctx, "pkgdc", "v", lvv{20})
			slog.Warn("pkgw", "v", lvv{21})
			slog.WarnContext(ctx, "pkgwc", "v", lvv{22})
			slog.Error("pkge", "v", lvv{23})
			slog.ErrorContext(ctx, "pkgec", "v", lvv{24})
			slog.Log(ctx, slog.LevelInfo, "pkglog", "v", lvv{25})
			slog.LogAttrs(ctx, slog.LevelInfo, "pkglogattrs", slog.Any("v", lvv{26}))
			slog.With("pw", lvv{27}).Info("pkgwith")
		})
		for _, x := range []any{lvv{1}, &lvp{"p"}, lvg[int]{2}, lvrec{2}, secret("s")} {
			out("Any %T kind=%v | AnyValue kind=%v", x, slog.Any("k", x).Value.Kind(), slog.AnyValue(x).Kind())
		}
	})
	// row: log/slog Attr.Equal,Attr.String,Value.Any,Value.Bool,Value.Duration,Value.Equal,Value.Float64,Value.Int64,Value.Kind,Value.LogValuer,Value.Resolve,Value.String,Value.Time,Value.Uint64 assert x.(log/slog.LogValuer)
	// row: log/slog Value.LogValuer,Value.Resolve panic x.(log/slog.LogValuer) without ok: *runtime.TypeAssertionError
	probe("Value", func() {
		for _, x := range []any{lvv{1}, &lvp{"p"}, lvg[string]{"g"}, lvrec{2}, lvloop{}, lvpanic{}, secret("s"), plain{1, "p"}, sv{2}} {
			v := slog.AnyValue(x)
			out("%T: kind=%v any=%T string=%q", x, v.Kind(), v.Any(), v.String())
			try("  Bool", func() any { return v.Bool() })
			try("  Duration", func() any { return v.Duration() })
			try("  Float64", func() any { return v.Float64() })
			try("  Int64", func() any { return v.Int64() })
			try("  Uint64", func() any { return v.Uint64() })
			try("  Time", func() any { return v.Time() })
			try("  LogValuer", func() any { return fmt.Sprintf("%T", v.LogValuer()) })
			try("  Equal", func() any { return v.Equal(slog.AnyValue(x)) })
			try("  Resolve", func() any {
				rv := v.Resolve()
				first, _, _ := strings.Cut(rv.String(), "\n")
				return fmt.Sprintf("kind=%v %q", rv.Kind(), first)
			})
			a := slog.Any("k", x)
			try("  Attr.String", func() any { return a.String() })
			try("  Attr.Equal", func() any { return a.Equal(slog.Any("k", x)) })
		}
		// LogValuer of a value that is not one: a type assertion panic
		try("LogValuer of int", func() any { return slog.IntValue(1).LogValuer() })
	})
	// row: log/slog Attr.Equal,Value.Equal panic == of two interfaces: comparing uncomparable type
	probe("Equal", func() {
		pairs := [][2]any{
			{plain{1, "a"}, plain{1, "a"}}, {plain{1, "a"}, plain{2, "a"}}, {I(1), 1}, {S("s"), "s"}, {sv{1}, sv{1}},
			{lvv{1}, lvv{1}}, {lvv{1}, lvv{2}}, {withSlice{}, plain{}}, {withSlice{}, withSlice{}},
			{[]int{1}, []int{1}}, {lvg[[]int]{}, lvg[[]int]{}},
		}
		for _, p := range pairs {
			try(fmt.Sprintf("Value.Equal %T %T", p[0], p[1]), func() any { return slog.AnyValue(p[0]).Equal(slog.AnyValue(p[1])) })
			try(fmt.Sprintf("Attr.Equal %T %T", p[0], p[1]), func() any { return slog.Any("k", p[0]).Equal(slog.Any("k", p[1])) })
		}
	})
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs,Attr.Equal,Attr.String,Value.Any,Value.Equal,Value.Resolve,Value.String,Value.Time panic panic message with the type name
	probe("Time", func() {
		for _, v := range []slog.Value{slog.TimeValue(fixed), slog.AnyValue(fixed), slog.TimeValue(time.Time{}), slog.AnyValue(fixed.In(time.FixedZone("Z2", 7200)))} {
			out("kind=%v time=%v any=%T string=%q resolve=%v equal=%t", v.Kind(), v.Time().UTC(), v.Any(), v.String(), v.Resolve().Kind(), v.Equal(slog.TimeValue(fixed)))
		}
		a := slog.Time("t", fixed)
		out("Attr.String %q Attr.Equal %t", a.String(), a.Equal(slog.Any("t", fixed)))
		both(func(l *slog.Logger, h slog.Handler) {
			l.Info("time", "t", fixed, "z", time.Time{})
			l.With("wt", fixed).Info("with")
			r := slog.NewRecord(time.Time{}, slog.LevelInfo, "record", 0)
			r.AddAttrs(slog.Time("rt", fixed))
			h.WithAttrs([]slog.Attr{slog.Time("wa", fixed)}).Handle(ctx, r)
		})
	})
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs assert *
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs panic x.(log/slog.LogValuer) without ok: *runtime.TypeAssertionError
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs reflect kind
	// row: log/slog (*JSONHandler).Handle,(*JSONHandler).WithAttrs,(*TextHandler).Handle,(*TextHandler).WithAttrs reflect walk
	probe("Handlers", func() {
		kv := []any{
			"tm", tm{1}, "tmp", &tmp{2}, "tmpv", tmp{3}, "tmErr", tmErr{}, "tmPanic", tmPanic{}, "nilTm", (*tm)(nil), "nilTmp", (*tmp)(nil),
			"jm", jm{4}, "jmp", &jmp{5}, "jmpv", jmp{6}, "errJM", errJM{}, "ev", ev{"v"}, "ep", &ep{"p"}, "nilEv", (*ev)(nil),
			"ta", ta{7}, "tmta", tmta{}, "sv", sv{8}, "plain", plain{9, "b"}, "bs", Bs("by"), "bs8", Bs8{104, 105}, "nilBs", Bs(nil),
			"slice", []any{tm{10}, ev{"x"}, I(11)}, "map", map[string]any{"k": jm{12}}, "iface", error(ev{"i"}),
		}
		both(func(l *slog.Logger, h slog.Handler) {
			l.Info("handle", kv...)
			l.With(kv...).Info("withattrs")
			l.WithGroup("grp").With("tm", tm{13}).Info("withgroup", "ev", ev{"g"})
			r := slog.NewRecord(time.Time{}, slog.LevelWarn, "record", 0)
			r.Add(kv...)
			h.Handle(ctx, r)
		})
	})
}
