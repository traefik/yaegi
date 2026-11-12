// reflect on program types: TypeOf(x).String(), Name, Kind, NumField,
// Field(i).Name/Type/Tag/Anonymous/IsExported, NumMethod and method names
// on T and *T, Elem, Implements, ConvertibleTo, and Value reads of fields.
package main

import (
	"fmt"
	"reflect"
)

type Inner struct {
	Z int
}

type User struct {
	Name  string `json:"name" db:"user_name"`
	Age   int    `json:"age,omitempty"`
	email string
	Inner
	Tags []string       `json:"tags"`
	Meta map[string]int `json:"-"`
	Next *User
	F    func(int) error
	Ch   chan<- bool
	Arr  [2]float32
	Any  any
	Err  error
}

func (u User) Hello() string     { return "hi " + u.Name }
func (u User) Older(n int) User  { u.Age += n; return u }
func (u *User) SetName(n string) { u.Name = n }
func (u User) private()          {}

type Level uint8

func (l Level) String() string { return fmt.Sprint("L", uint8(l)) }

type Greeter interface {
	Hello() string
}

func methods(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		out = append(out, m.Name+" "+m.Type.String())
	}
	return out
}

func main() {
	u := User{Name: "ann", Age: 30, email: "x", Inner: Inner{7}, Tags: []string{"a"}}
	t := reflect.TypeOf(u)
	fmt.Println(t.String(), t.Name(), t.Kind(), t.NumField(), t.PkgPath())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Printf("%d %s %s %q json=%q db=%q anon=%v exported=%v kind=%v idx=%v\n",
			i, f.Name, f.Type, f.Tag, f.Tag.Get("json"), f.Tag.Get("db"), f.Anonymous, f.IsExported(), f.Type.Kind(), f.Index)
	}
	if f, ok := t.FieldByName("Z"); ok {
		fmt.Println("promoted Z index", f.Index)
	}
	_, ok := t.FieldByName("missing")
	fmt.Println(ok)
	v, ok := t.Field(0).Tag.Lookup("db")
	fmt.Println(v, ok)

	fmt.Println(t.NumMethod(), methods(t))
	pt := reflect.TypeOf(&u)
	fmt.Println(pt.String(), pt.Kind(), pt.Name() == "", pt.Elem() == t, pt.NumMethod(), methods(pt))

	gi := reflect.TypeOf((*Greeter)(nil)).Elem()
	st := reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	fmt.Println(gi.String(), gi.Kind(), gi.NumMethod(), t.Implements(gi), pt.Implements(gi), t.Implements(st))
	lt := reflect.TypeOf(Level(3))
	fmt.Println(lt.String(), lt.Kind(), lt.Implements(st), lt.ConvertibleTo(reflect.TypeOf(0)), lt.ConvertibleTo(reflect.TypeOf("")), lt.ConvertibleTo(t))

	for _, x := range []any{
		0, int8(0), uint64(0), 1.5, float32(1), "s", true, 'r', byte(1), complex(1, 1),
		[]int{}, [3]string{}, map[string][]int{}, make(chan int), make(<-chan string), func(int, ...string) (bool, error) { return false, nil },
		&u, struct{ A, b int }{}, struct{}{}, Level(1), []User{}, map[Level]*User{}, new(any), new(error),
	} {
		rt := reflect.TypeOf(x)
		fmt.Printf("%s | %v | %q\n", rt.String(), rt.Kind(), rt.Name())
	}
	fmt.Println(reflect.TypeOf(nil) == nil)

	ft := reflect.TypeOf(u.Older)
	fmt.Println(ft.String(), ft.NumIn(), ft.NumOut(), ft.In(0), ft.Out(0))
	vt := reflect.TypeOf(fmt.Sprintf)
	fmt.Println(vt.String(), vt.IsVariadic(), vt.In(1))
	mt := reflect.TypeOf(map[string][]int{})
	fmt.Println(mt.Key(), mt.Elem(), mt.Elem().Elem())
	at := reflect.TypeOf([4]byte{})
	fmt.Println(at.Len(), at.Elem(), at.Size())
	ct := reflect.TypeOf(make(chan<- bool))
	fmt.Println(ct.ChanDir(), ct.Elem())

	// Values.
	rv := reflect.ValueOf(u)
	fmt.Println(rv.Kind(), rv.NumField(), rv.Field(0), rv.Field(1).Int(), rv.Field(3).Field(0).Int(), rv.FieldByName("Z"))
	fmt.Println(rv.Field(2).String(), rv.Field(2).CanInterface(), rv.Field(0).CanInterface(), rv.CanSet())
	fmt.Println(rv.Field(4).Len(), rv.Field(4).Index(0), rv.Field(5).IsNil(), rv.Field(6).IsNil(), rv.Field(10).IsNil())
	res := rv.MethodByName("Hello").Call(nil)
	fmt.Println(res[0].String(), rv.NumMethod())
	res = rv.MethodByName("Older").Call([]reflect.Value{reflect.ValueOf(5)})
	fmt.Println(res[0].Field(1).Int(), u.Age)
	fmt.Println(reflect.ValueOf(Level(4)).Interface(), reflect.ValueOf(Level(4)).Uint())
	fmt.Println(reflect.Zero(t).Field(0).String() == "", reflect.New(t).Elem().Field(1).Int())
}
