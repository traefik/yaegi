package main

import (
	"fmt"
	"reflect"
)

type Cfg struct {
	Name string
	Port int
}

func (c *Cfg) Addr() string { return fmt.Sprintf("%s:%d", c.Name, c.Port) }

type Addrer interface{ Addr() string }

func main() {
	v := reflect.New(reflect.TypeOf(Cfg{}))
	v.Elem().Field(0).SetString("h")
	v.Elem().FieldByName("Port").SetInt(80)
	c := v.Interface().(*Cfg)
	fmt.Println(c.Addr())
	a, ok := v.Interface().(Addrer)
	fmt.Println(ok, a != nil)
	z := reflect.Zero(reflect.TypeOf(Cfg{})).Interface().(Cfg)
	fmt.Printf("%+v\n", z)
	st := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(""), Tag: `json:"a"`},
		{Name: "C", Type: reflect.TypeOf(Cfg{})},
	})
	sv := reflect.New(st).Elem()
	sv.Field(0).SetString("x")
	sv.Field(1).Set(reflect.ValueOf(Cfg{"n", 1}))
	fmt.Printf("%+v %v\n", sv.Interface(), st.Field(0).Tag.Get("json"))
	mt := reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(Cfg{}))
	mv := reflect.MakeMap(mt)
	mv.SetMapIndex(reflect.ValueOf("k"), reflect.ValueOf(Cfg{"m", 2}))
	fmt.Println(mv.Interface().(map[string]Cfg)["k"].Port)
}
