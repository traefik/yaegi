// AOT test program: github.com/mitchellh/mapstructure on interpreted types.
//
//aot:lib github.com/mitchellh/mapstructure
package main

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

type Level int

type Endpoint struct {
	Host string
	Port int
}

type Base struct {
	ID   int    `mapstructure:"id"`
	Kind string `mapstructure:"kind"`
}

type Config struct {
	Base      `mapstructure:",squash"`
	Name      string            `mapstructure:"name"`
	Level     Level             `mapstructure:"level"`
	Timeout   time.Duration     `mapstructure:"timeout"`
	Endpoints []Endpoint        `mapstructure:"endpoints"`
	Labels    map[string]string `mapstructure:"labels"`
	Main      *Endpoint         `mapstructure:"main"`
	Any       any               `mapstructure:"any"`
	Tags      []string          `mapstructure:"tags"`
	Rest      map[string]any    `mapstructure:",remain"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	in := map[string]any{
		"id": 3, "kind": "svc", "name": "api", "level": "2", "timeout": "1m",
		"endpoints": []any{map[string]any{"Host": "a", "Port": 1}, map[string]any{"host": "b", "port": "2"}},
		"labels":    map[string]any{"x": "1"}, "main": map[string]any{"Host": "m"},
		"any": []int{1, 2}, "tags": "a,b,c", "extra": true,
	}
	var c Config
	var md mapstructure.Metadata
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &c, Metadata: &md, WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			func(f reflect.Type, t reflect.Type, data any) (any, error) {
				if t == reflect.TypeOf(Level(0)) && f.Kind() == reflect.String {
					n, err := strconv.Atoi(data.(string))
					return Level(n * 10), err
				}
				return data, nil
			},
		),
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	err = dec.Decode(in)
	fmt.Println(err)
	fmt.Printf("%+v\n", c.Base)
	fmt.Println(c.Name, c.Level, c.Timeout, c.Endpoints, c.Labels, *c.Main, c.Any, c.Tags, c.Rest)
	fmt.Println(strings.Join(md.Keys, ","), md.Unused)
	var out map[string]any
	err = mapstructure.Decode(Endpoint{"h", 9}, &out)
	fmt.Println(err, out)
	var e Endpoint
	err = mapstructure.Decode(map[string]any{"Port": "x"}, &e)
	fmt.Println(err)
	err = mapstructure.WeakDecode(map[string]any{"Port": "42", "Host": 7}, &e)
	fmt.Println(err, e)
	var lv []Level
	err = mapstructure.Decode([]any{1, 2.0, uint8(3)}, &lv)
	fmt.Println(err, lv)
	var em *mapstructure.Error
	err = mapstructure.Decode(map[string]any{"Port": []int{1}, "Host": 1}, &e)
	_, isErr := err.(*mapstructure.Error)
	fmt.Println(isErr, em == nil)
}

// benchOp: decode a map into an interpreted struct.
func benchOp() {
	in := map[string]any{"id": 3, "kind": "svc", "name": "api", "level": 2,
		"endpoints": []any{map[string]any{"Host": "a", "Port": 1}}, "labels": map[string]any{"x": "1"}}
	var c Config
	if err := mapstructure.Decode(in, &c); err != nil {
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
