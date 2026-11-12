package main

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

type Upper string

type Cfg struct {
	Name    string        `mapstructure:"name"`
	Timeout time.Duration `mapstructure:"timeout"`
	Who     Upper         `mapstructure:"who"`
	Ports   []int         `mapstructure:"ports"`
	Extra   map[string]any `mapstructure:",remain"`
}

func main() {
	var c Cfg
	hook := func(f, t reflect.Type, data any) (any, error) {
		if t == reflect.TypeOf(Upper("")) {
			return Upper(strings.ToUpper(data.(string))), nil
		}
		return data, nil
	}
	d, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &c,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(mapstructure.StringToTimeDurationHookFunc(), hook),
	})
	err := d.Decode(map[string]any{"name": "n", "timeout": "2s", "who": "bob", "ports": []any{80, 443}, "x": 1})
	fmt.Printf("%v %+v\n", err, c)
}
