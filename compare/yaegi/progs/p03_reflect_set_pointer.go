package main

import (
	"fmt"
	"reflect"
)

type Cfg struct {
	Name  string
	Ports []int
	Sub   *Sub
	M     map[string]int
	Any   any
}

type Sub struct{ Level int }

func set(dst any, field string, val any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer {
		return fmt.Errorf("not a pointer")
	}
	f := v.Elem().FieldByName(field)
	if !f.IsValid() || !f.CanSet() {
		return fmt.Errorf("cannot set %s", field)
	}
	nv := reflect.ValueOf(val)
	if !nv.Type().AssignableTo(f.Type()) {
		if nv.Type().ConvertibleTo(f.Type()) {
			nv = nv.Convert(f.Type())
		} else {
			return fmt.Errorf("type mismatch %s vs %s", nv.Type(), f.Type())
		}
	}
	f.Set(nv)
	return nil
}

func main() {
	c := &Cfg{}
	fmt.Println(set(c, "Name", "n"), set(c, "Ports", []int{1, 2}), set(c, "Sub", &Sub{3}), set(c, "M", map[string]int{"a": 1}))
	fmt.Println(set(c, "Any", 4.5), set(c, "Nope", 1), set(*c, "Name", "x"))
	fmt.Println(c.Name, c.Ports, c.Sub.Level, c.M, c.Any)
	rv := reflect.ValueOf(c).Elem()
	rv.Field(1).Index(0).SetInt(9)
	rv.FieldByName("Sub").Elem().Field(0).SetInt(7)
	rv.FieldByName("M").SetMapIndex(reflect.ValueOf("b"), reflect.ValueOf(2))
	rv.FieldByName("Ports").Set(reflect.Append(rv.FieldByName("Ports"), reflect.ValueOf(5)))
	fmt.Println(c.Ports, c.Sub.Level, len(c.M))
	var x int
	reflect.ValueOf(&x).Elem().SetInt(11)
	fmt.Println(x)
}
