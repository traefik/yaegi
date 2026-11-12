// AOT test program: github.com/go-viper/mapstructure/v2 on interpreted types.
//
//aot:lib github.com/go-viper/mapstructure/v2
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

type Mode string

// Port implements encoding.TextUnmarshaler (used by TextUnmarshallerHookFunc).
type Port int

func (p *Port) UnmarshalText(b []byte) error {
	n, err := strconv.Atoi(string(b))
	*p = Port(n + 1000)
	return err
}

type Inner struct {
	A int    `mapstructure:"a"`
	B string `mapstructure:"b,omitempty"`
}

type Settings struct {
	Mode    Mode              `mapstructure:"mode"`
	Port    Port              `mapstructure:"port"`
	Wait    time.Duration     `mapstructure:"wait"`
	Inner   Inner             `mapstructure:"inner"`
	List    []Inner           `mapstructure:"list"`
	Map     map[string]Inner  `mapstructure:"map"`
	PtrIn   *Inner            `mapstructure:"ptr"`
	Ints    []int             `mapstructure:"ints"`
	Flags   map[string]bool   `mapstructure:"flags"`
	Unknown map[string]any    `mapstructure:",remain"`
	Labels  map[string]string `mapstructure:"labels"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	in := map[string]any{
		"mode": "fast", "port": "80", "wait": "2s",
		"inner": map[string]any{"a": 1, "b": "x"},
		"list":  []map[string]any{{"a": 2}, {"a": 3, "b": "y"}},
		"map":   map[string]any{"k": map[string]any{"a": 4}},
		"ptr":   map[string]any{"a": 5},
		"ints":  []any{1, 2, 3},
		"flags": map[string]any{"on": true},
		"zzz":   1,
	}
	var s Settings
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &s,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.TextUnmarshallerHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
		),
		ErrorUnused: false,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	err = dec.Decode(in)
	fmt.Println(err)
	p := s.PtrIn // printed by value: an address differs between runs
	s.PtrIn = nil
	fmt.Printf("%+v %v\n", s, p != nil)
	if p != nil {
		fmt.Printf("%+v\n", *p)
	}
	var back map[string]any
	err = mapstructure.Decode(Inner{A: 7, B: "z"}, &back)
	fmt.Println(err, back)
	var bad Inner
	err = mapstructure.Decode(map[string]any{"a": "nope"}, &bad)
	fmt.Println(err)
	var sl []Port
	err = mapstructure.Decode([]int{1, 2}, &sl)
	fmt.Println(err, sl)
}

// benchOp: decode a map into an interpreted struct.
func benchOp() {
	in := map[string]any{"mode": "fast", "inner": map[string]any{"a": 1, "b": "x"},
		"list": []map[string]any{{"a": 2}}, "ints": []any{1, 2, 3}}
	var s Settings
	if err := mapstructure.Decode(in, &s); err != nil {
		panic(err)
	}
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
