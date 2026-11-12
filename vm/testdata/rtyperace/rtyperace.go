// Package rtyperace is a plugin whose handlers each box one of a few types
// into interfaces for the first time, then call methods through interfaces,
// type switches and native code (fmt calls String). Functions compile at
// their first call, so concurrent first requests build the run-time type
// descriptors of these types at the same time.
package rtyperace

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type Namer interface{ Name() string }

type Sizer interface{ Size() int }

// use calls the methods of x through interfaces, a type switch and fmt.
func use(w io.Writer, x any) {
	n := x.(Namer).Name()
	s := 0
	if z, ok := x.(Sizer); ok {
		s = z.Size()
	}
	kind := "?"
	switch x.(type) {
	case T0:
		kind = "v0"
	case *T0:
		kind = "p0"
	case T1:
		kind = "v1"
	case *T1:
		kind = "p1"
	case T2:
		kind = "v2"
	case *T2:
		kind = "p2"
	case T3:
		kind = "v3"
	case *T3:
		kind = "p3"
	case T4:
		kind = "v4"
	case *T4:
		kind = "p4"
	case T5:
		kind = "v5"
	case *T5:
		kind = "p5"
	case T6:
		kind = "v6"
	case *T6:
		kind = "p6"
	case T7:
		kind = "v7"
	case *T7:
		kind = "p7"
	}
	fmt.Fprintf(w, "%s %d %s %v;", n, s, kind, x)
}

type T0 struct{ v int }

func (t T0) Name() string    { return "t0:" + strconv.Itoa(t.v) }
func (t T0) String() string  { return "<0:" + strconv.Itoa(t.v) + ">" }
func (t *T0) Size() int      { return t.v * 1 }
func (t *T0) Grow(n int) int { t.v += n; return t.v }

type T1 struct{ v int }

func (t T1) Name() string    { return "t1:" + strconv.Itoa(t.v) }
func (t T1) String() string  { return "<1:" + strconv.Itoa(t.v) + ">" }
func (t *T1) Size() int      { return t.v * 2 }
func (t *T1) Grow(n int) int { t.v += n; return t.v }

type T2 struct{ v int }

func (t T2) Name() string    { return "t2:" + strconv.Itoa(t.v) }
func (t T2) String() string  { return "<2:" + strconv.Itoa(t.v) + ">" }
func (t *T2) Size() int      { return t.v * 3 }
func (t *T2) Grow(n int) int { t.v += n; return t.v }

type T3 struct{ v int }

func (t T3) Name() string    { return "t3:" + strconv.Itoa(t.v) }
func (t T3) String() string  { return "<3:" + strconv.Itoa(t.v) + ">" }
func (t *T3) Size() int      { return t.v * 4 }
func (t *T3) Grow(n int) int { t.v += n; return t.v }

type T4 struct{ v int }

func (t T4) Name() string    { return "t4:" + strconv.Itoa(t.v) }
func (t T4) String() string  { return "<4:" + strconv.Itoa(t.v) + ">" }
func (t *T4) Size() int      { return t.v * 5 }
func (t *T4) Grow(n int) int { t.v += n; return t.v }

type T5 struct{ v int }

func (t T5) Name() string    { return "t5:" + strconv.Itoa(t.v) }
func (t T5) String() string  { return "<5:" + strconv.Itoa(t.v) + ">" }
func (t *T5) Size() int      { return t.v * 6 }
func (t *T5) Grow(n int) int { t.v += n; return t.v }

type T6 struct{ v int }

func (t T6) Name() string    { return "t6:" + strconv.Itoa(t.v) }
func (t T6) String() string  { return "<6:" + strconv.Itoa(t.v) + ">" }
func (t *T6) Size() int      { return t.v * 7 }
func (t *T6) Grow(n int) int { t.v += n; return t.v }

type T7 struct{ v int }

func (t T7) Name() string    { return "t7:" + strconv.Itoa(t.v) }
func (t T7) String() string  { return "<7:" + strconv.Itoa(t.v) + ">" }
func (t *T7) Size() int      { return t.v * 8 }
func (t *T7) Grow(n int) int { t.v += n; return t.v }

func h0(w io.Writer) {
	var x any = T0{0}
	use(w, x)
}

func h1(w io.Writer) {
	var x any = T1{1}
	use(w, x)
}

func h2(w io.Writer) {
	var x any = T2{2}
	use(w, x)
}

func h3(w io.Writer) {
	var x any = T3{3}
	use(w, x)
}

func h4(w io.Writer) {
	var x any = T4{4}
	use(w, x)
}

func h5(w io.Writer) {
	var x any = T5{5}
	use(w, x)
}

func h6(w io.Writer) {
	var x any = T6{6}
	use(w, x)
}

func h7(w io.Writer) {
	var x any = T7{7}
	use(w, x)
}

func h8(w io.Writer) {
	p := &T0{8}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h9(w io.Writer) {
	p := &T1{9}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h10(w io.Writer) {
	p := &T2{10}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h11(w io.Writer) {
	p := &T3{11}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h12(w io.Writer) {
	p := &T4{12}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h13(w io.Writer) {
	p := &T5{13}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h14(w io.Writer) {
	p := &T6{14}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h15(w io.Writer) {
	p := &T7{15}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h16(w io.Writer) {
	var x any = T0{16}
	use(w, x)
}

func h17(w io.Writer) {
	var x any = T1{17}
	use(w, x)
}

func h18(w io.Writer) {
	var x any = T2{18}
	use(w, x)
}

func h19(w io.Writer) {
	var x any = T3{19}
	use(w, x)
}

func h20(w io.Writer) {
	var x any = T4{20}
	use(w, x)
}

func h21(w io.Writer) {
	var x any = T5{21}
	use(w, x)
}

func h22(w io.Writer) {
	var x any = T6{22}
	use(w, x)
}

func h23(w io.Writer) {
	var x any = T7{23}
	use(w, x)
}

func h24(w io.Writer) {
	p := &T0{24}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h25(w io.Writer) {
	p := &T1{25}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h26(w io.Writer) {
	p := &T2{26}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h27(w io.Writer) {
	p := &T3{27}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h28(w io.Writer) {
	p := &T4{28}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h29(w io.Writer) {
	p := &T5{29}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h30(w io.Writer) {
	p := &T6{30}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h31(w io.Writer) {
	p := &T7{31}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h32(w io.Writer) {
	var x any = T0{32}
	use(w, x)
}

func h33(w io.Writer) {
	var x any = T1{33}
	use(w, x)
}

func h34(w io.Writer) {
	var x any = T2{34}
	use(w, x)
}

func h35(w io.Writer) {
	var x any = T3{35}
	use(w, x)
}

func h36(w io.Writer) {
	var x any = T4{36}
	use(w, x)
}

func h37(w io.Writer) {
	var x any = T5{37}
	use(w, x)
}

func h38(w io.Writer) {
	var x any = T6{38}
	use(w, x)
}

func h39(w io.Writer) {
	var x any = T7{39}
	use(w, x)
}

func h40(w io.Writer) {
	p := &T0{40}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h41(w io.Writer) {
	p := &T1{41}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h42(w io.Writer) {
	p := &T2{42}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h43(w io.Writer) {
	p := &T3{43}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h44(w io.Writer) {
	p := &T4{44}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h45(w io.Writer) {
	p := &T5{45}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h46(w io.Writer) {
	p := &T6{46}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h47(w io.Writer) {
	p := &T7{47}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h48(w io.Writer) {
	var x any = T0{48}
	use(w, x)
}

func h49(w io.Writer) {
	var x any = T1{49}
	use(w, x)
}

func h50(w io.Writer) {
	var x any = T2{50}
	use(w, x)
}

func h51(w io.Writer) {
	var x any = T3{51}
	use(w, x)
}

func h52(w io.Writer) {
	var x any = T4{52}
	use(w, x)
}

func h53(w io.Writer) {
	var x any = T5{53}
	use(w, x)
}

func h54(w io.Writer) {
	var x any = T6{54}
	use(w, x)
}

func h55(w io.Writer) {
	var x any = T7{55}
	use(w, x)
}

func h56(w io.Writer) {
	p := &T0{56}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h57(w io.Writer) {
	p := &T1{57}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h58(w io.Writer) {
	p := &T2{58}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h59(w io.Writer) {
	p := &T3{59}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h60(w io.Writer) {
	p := &T4{60}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h61(w io.Writer) {
	p := &T5{61}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h62(w io.Writer) {
	p := &T6{62}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

func h63(w io.Writer) {
	p := &T7{63}
	var g interface{ Grow(int) int } = p
	g.Grow(1)
	use(w, p)
}

var handlers = []func(io.Writer){h0, h1, h2, h3, h4, h5, h6, h7, h8, h9, h10, h11, h12, h13, h14, h15, h16, h17, h18, h19, h20, h21, h22, h23, h24, h25, h26, h27, h28, h29, h30, h31, h32, h33, h34, h35, h36, h37, h38, h39, h40, h41, h42, h43, h44, h45, h46, h47, h48, h49, h50, h51, h52, h53, h54, h55, h56, h57, h58, h59, h60, h61, h62, h63}

// New returns a handler that runs handlers[N] for the path /N.
func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil || i < 0 || i >= len(handlers) {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		handlers[i](w)
	}), nil
}
