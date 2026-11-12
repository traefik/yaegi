package main

import (
	"fmt"
	"reflect"
	"strconv"
)

type Inner struct {
	Port int `cfg:"port" default:"8080"`
}

type Config struct {
	Name    string            `cfg:"name" default:"svc"`
	Enabled bool              `cfg:"enabled" default:"true"`
	Ratio   float64           `cfg:"ratio" default:"0.5"`
	Inner                     // embedded
	Tags    []string          `cfg:"tags"`
	Labels  map[string]string `cfg:"labels"`
	hidden  int
}

func applyDefaults(v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			applyDefaults(fv)
			continue
		}
		if !f.IsExported() {
			continue
		}
		d, ok := f.Tag.Lookup("default")
		if !ok || !fv.IsZero() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fv.SetString(d)
		case reflect.Bool:
			b, _ := strconv.ParseBool(d)
			fv.SetBool(b)
		case reflect.Int:
			n, _ := strconv.Atoi(d)
			fv.SetInt(int64(n))
		case reflect.Float64:
			x, _ := strconv.ParseFloat(d, 64)
			fv.SetFloat(x)
		}
	}
}

func main() {
	c := &Config{Name: "given"}
	applyDefaults(reflect.ValueOf(c).Elem())
	fmt.Printf("%+v\n", *c)
	t := reflect.TypeOf(*c)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Println(f.Name, f.Type, f.Tag.Get("cfg"), f.Anonymous, f.IsExported(), f.Index)
	}
	sf, ok := t.FieldByName("Port")
	fmt.Println(ok, sf.Index, sf.Tag.Get("default"))
	fmt.Println(t.Name(), t.NumField(), t.Kind())
}
