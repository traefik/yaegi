// github.com/mitchellh/mapstructure as Traefik decodes a plugin
// configuration (pkg/plugins: DecodeConfig): WeaklyTypedInput, a decode
// hook for comma-separated strings into slices, squash, nested structs,
// pointers, maps, interfaces, Metadata and errors.
package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

type Base struct {
	Enabled bool `json:"enabled,omitempty"`
}

type Header struct {
	Name  string
	Value string
}

type Config struct {
	Base      `mapstructure:",squash"`
	Prefix    string
	Methods   []string
	Ports     []int
	Max       int
	Rate      float64
	Timeout   time.Duration
	Headers   []Header
	ByName    map[string]Header
	Ptr       *Header
	Any       any
	Level     Level
	Rules     map[string][]string
	NotInMap  string
	unexpored int
}

type Level int

func stringToSliceHook(f reflect.Kind, t reflect.Kind, data any) (any, error) {
	if f != reflect.String || t != reflect.Slice {
		return data, nil
	}
	raw := data.(string)
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, ","), nil
}

func decode(in map[string]any, cfg *Config) (mapstructure.Metadata, error) {
	var md mapstructure.Metadata
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(stringToSliceHook, mapstructure.StringToTimeDurationHookFunc()),
		WeaklyTypedInput: true,
		Result:           cfg,
		Metadata:         &md,
	})
	if err != nil {
		return md, err
	}
	err = dec.Decode(in)
	return md, err
}

func main() {
	in := map[string]any{
		"enabled": "true",
		"prefix":  "/api",
		"methods": "GET,POST",
		"ports":   []any{"80", 443},
		"max":     "12",
		"rate":    "0.5",
		"timeout": "3s",
		"headers": []any{map[string]any{"name": "X-A", "value": "1"}, map[string]any{"Name": "X-B", "Value": 2}},
		"byName":  map[string]any{"h": map[string]any{"name": "n", "value": "v"}},
		"ptr":     map[string]any{"name": "p"},
		"any":     map[string]any{"deep": []any{1, "x"}},
		"level":   "3",
		"rules":   map[string]any{"r1": "a,b", "r2": []any{"c"}},
		"unknown": 1,
	}
	cfg := &Config{Max: 1, NotInMap: "default"}
	md, err := decode(in, cfg)
	fmt.Println(err)
	fmt.Printf("%+v\n", cfg.Ptr)
	fmt.Println(cfg.Enabled, cfg.Prefix, cfg.Methods, len(cfg.Methods), cfg.Ports, cfg.Max, cfg.Rate, cfg.Timeout, cfg.Level, cfg.NotInMap)
	fmt.Println(cfg.Headers, cfg.ByName, cfg.Any, cfg.Rules)
	sort.Strings(md.Keys)
	sort.Strings(md.Unused)
	fmt.Println(md.Keys, md.Unused)

	var c2 Config
	_, err = decode(map[string]any{"max": "notanumber", "ptr": "x", "headers": 5}, &c2)
	fmt.Println(err)

	// Decoding back to a map.
	var out map[string]any
	err = mapstructure.Decode(Header{"k", "v"}, &out)
	fmt.Println(err, out)
}
