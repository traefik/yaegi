// AOT test program: github.com/spf13/cast on interpreted types.
//
//aot:lib github.com/spf13/cast
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cast"
)

type Level int

func (l Level) String() string { return "level" + strconv.Itoa(int(l)) }

type Ratio float32

type Name string

type Flag bool

type MyErr struct{}

func (MyErr) Error() string { return "my error" }

func main() {
	if benchMain(benchOp) {
		return
	}
	fmt.Println(cast.ToString(Level(2)), cast.ToString(Name("n")), cast.ToString(Ratio(1.5)), cast.ToString(MyErr{}), cast.ToString([]byte("b")))
	fmt.Println(cast.ToInt("42"), cast.ToInt(Level(3)), cast.ToInt(Ratio(2.9)), cast.ToInt("0x1f"), cast.ToInt(Name("7")))
	fmt.Println(cast.ToBool("true"), cast.ToBool(Flag(true)), cast.ToBool(1), cast.ToFloat64(Ratio(0.25)), cast.ToFloat64("1e3"))
	n, err := cast.ToIntE("x")
	fmt.Println(n, err)
	fmt.Println(cast.ToStringSlice([]any{"a", Level(1), 3}), cast.ToIntSlice([]any{"1", 2, Level(3)}))
	fmt.Println(cast.ToStringMap(map[string]any{"a": Level(1)}), cast.ToStringMapString(map[any]any{"k": Name("v"), 1: 2}))
	fmt.Println(cast.ToStringMapInt(map[string]Level{"x": 4}), cast.ToStringMapBool(`{"on": true}`))
	fmt.Println(cast.ToDuration("1m30s"), cast.ToDuration(Level(5)), cast.ToTime("2024-01-02T03:04:05Z").Year())
	fmt.Println(cast.ToSlice([]Level{1, 2}), cast.ToUint8(Level(300)), cast.ToInt64(new(int)))
	var p *Level
	fmt.Println(cast.ToInt(p), cast.ToString(nil), cast.ToString(&[]int{1}))
	_, err = cast.ToStringE(struct{ A int }{1})
	fmt.Println(err != nil, errors.Is(err, err))
}

// benchOp: a few conversions of interpreted values.
func benchOp() {
	_ = cast.ToString(Level(2)) + cast.ToString(Name("n"))
	_ = cast.ToInt(Level(3)) + cast.ToInt("42")
	_ = cast.ToStringSlice([]any{"a", Level(1), 3})
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
