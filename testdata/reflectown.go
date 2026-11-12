package main

// reflect called by the interpreted program itself, beyond the VM reflect
// intrinsics (run with -aot by TestTestdata): the calls go to the twin.

import (
	"fmt"
	"reflect"
	"strings"
)

type Inner struct{ Port int }

type Config struct {
	Name    string `cfg:"name"`
	Enabled bool
	Ratio   float64
	Inner   Inner
	Tags    []string
	secret  int
}

func (c Config) Describe(prefix string) string { return prefix + c.Name }
func (c *Config) Rename(n string)              { c.Name = n }

type Describer interface{ Describe(string) string }

func main() {
	c := &Config{Name: "a", secret: 1}
	v := reflect.Indirect(reflect.ValueOf(c))
	v.FieldByName("Name").SetString("given")
	v.FieldByName("Enabled").SetBool(true)
	v.FieldByName("Ratio").SetFloat(0.5)
	v.FieldByName("Inner").FieldByName("Port").SetInt(8080)
	v.FieldByName("Tags").Set(reflect.ValueOf([]string{"x", "y"}))
	fmt.Printf("%+v\n", *c)
	f, ok := v.Type().FieldByName("Name")
	fmt.Println(ok, f.Tag.Get("cfg"), f.Index, v.Type().PkgPath(), v.Type().String())
	fmt.Println(v.FieldByName("secret").CanSet(), v.FieldByName("missing").IsValid())

	pt := reflect.TypeOf(c)
	var names []string
	for i := 0; i < pt.NumMethod(); i++ {
		names = append(names, pt.Method(i).Name)
	}
	fmt.Println(strings.Join(names, ","), pt.Implements(reflect.TypeOf((*Describer)(nil)).Elem()))
	reflect.ValueOf(c).MethodByName("Rename").Call([]reflect.Value{reflect.ValueOf("renamed")})
	out := reflect.ValueOf(*c).MethodByName("Describe").Call([]reflect.Value{reflect.ValueOf("> ")})
	fmt.Println(out[0].String())
	m, ok := pt.MethodByName("Describe")
	fmt.Println(ok, m.Func.Call([]reflect.Value{reflect.ValueOf(c), reflect.ValueOf("! ")})[0])

	fmt.Println(reflect.DeepEqual(Config{Name: "n", Tags: []string{"t"}}, Config{Name: "n", Tags: []string{"t"}}),
		reflect.DeepEqual(map[string]Inner{"a": {1}}, map[string]Inner{"a": {2}}))

	st := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(""), Tag: `json:"a"`},
		{Name: "In", Type: reflect.TypeOf(Inner{})},
	})
	sv := reflect.New(st).Elem()
	sv.Field(0).SetString("s")
	sv.Field(1).Set(reflect.ValueOf(Inner{3}))
	fmt.Printf("%+v %s %d\n", sv.Interface(), st.Field(0).Tag.Get("json"), st.NumField())

	mt := reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(Inner{}))
	mv := reflect.MakeMap(mt)
	mv.SetMapIndex(reflect.ValueOf("k"), reflect.ValueOf(Inner{4}))
	fmt.Println(mv.Interface().(map[string]Inner)["k"].Port, reflect.Zero(reflect.TypeOf(Inner{})).Interface().(Inner))
}
