// reflect on interpreted types, called by the program itself: Type names
// and kinds, struct fields with tags and embedded fields, FieldByName
// through embedding, Set* on addressable values, CanSet on unexported
// fields, MapRange and SetMapIndex, Append and MakeSlice, Convert,
// Method and MethodByName with Call, Implements, New and Zero, DeepEqual,
// and reflect.TypeFor.
package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type Celsius float64

func (c Celsius) String() string        { return fmt.Sprintf("%.1fC", float64(c)) }
func (c Celsius) Add(d float64) Celsius { return c + Celsius(d) }
func (c *Celsius) Reset()               { *c = 0 }

type Audit struct {
	CreatedBy string `db:"created_by" json:"createdBy,omitempty"`
}

type Config struct {
	Audit
	Name    string         `db:"name" validate:"required"`
	Port    int            `db:"port"`
	Ratio   float64        `db:"-"`
	Tags    []string       `db:"tags"`
	Limits  map[string]int `db:"limits"`
	Temp    Celsius        `db:"temp"`
	Next    *Config        `db:"next"`
	private string
}

type Greeter interface{ Greet(string) string }

type English struct{ Excl bool }

func (e English) Greet(n string) string {
	if e.Excl {
		return "Hello, " + n + "!"
	}
	return "Hello, " + n
}

func main() {
	c := Config{Audit: Audit{"root"}, Name: "svc", Port: 80, Tags: []string{"a"}, Limits: map[string]int{"x": 1}, Temp: 21.5, private: "p"}
	t := reflect.TypeOf(c)
	fmt.Println(t.Name(), t.Kind(), t.NumField(), t.String())
	for i := range t.NumField() {
		f := t.Field(i)
		fmt.Printf("%s %s anon=%v exported=%v db=%q validate=%q\n", f.Name, f.Type, f.Anonymous, f.IsExported(), f.Tag.Get("db"), f.Tag.Get("validate"))
	}
	f, ok := t.FieldByName("CreatedBy")
	fmt.Println(f.Name, f.Index, ok, f.Tag.Get("json"))
	v, _ := f.Tag.Lookup("missing")
	fmt.Printf("%q\n", v)

	// Set through a pointer.
	rv := reflect.ValueOf(&c).Elem()
	rv.FieldByName("Name").SetString("renamed")
	rv.FieldByName("Port").SetInt(8080)
	rv.FieldByName("Ratio").SetFloat(0.5)
	rv.FieldByName("CreatedBy").SetString("admin")
	rv.FieldByName("Temp").Set(reflect.ValueOf(Celsius(30)))
	rv.FieldByName("Tags").Set(reflect.Append(rv.FieldByName("Tags"), reflect.ValueOf("b")))
	rv.FieldByName("Limits").SetMapIndex(reflect.ValueOf("y"), reflect.ValueOf(2))
	rv.FieldByName("Next").Set(reflect.New(t))
	rv.FieldByName("Next").Elem().FieldByName("Port").SetInt(1)
	fmt.Println(c.Name, c.Port, c.Ratio, c.CreatedBy, c.Temp, c.Tags, c.Limits, c.Next.Port)
	fmt.Println(rv.FieldByName("private").CanSet(), rv.FieldByName("Name").CanSet(), reflect.ValueOf(c).Field(1).CanSet())

	// Set* on a value that cannot be set panics.
	unset := reflect.ValueOf(c).Field(1)
	fmt.Println(unset.CanSet(), unset.CanAddr())
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		unset.SetString("x")
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		reflect.ValueOf(3).SetInt(4)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		rv.FieldByName("private").SetString("x")
	}()
	fmt.Println(c.Name, c.private)

	// MapRange in key order.
	lim := reflect.ValueOf(c.Limits)
	var keys []string
	it := lim.MapRange()
	for it.Next() {
		keys = append(keys, fmt.Sprint(it.Key(), "=", it.Value()))
	}
	sort.Strings(keys)
	fmt.Println(keys, lim.Len(), lim.MapIndex(reflect.ValueOf("nope")).IsValid())
	lim.SetMapIndex(reflect.ValueOf("x"), reflect.Value{}) // delete
	fmt.Println(c.Limits)

	// MakeSlice, MakeMap, Index.
	s := reflect.MakeSlice(reflect.TypeOf([]Celsius{}), 2, 4)
	s.Index(1).Set(reflect.ValueOf(Celsius(5)))
	s = reflect.Append(s, reflect.ValueOf(Celsius(7)))
	fmt.Println(s.Interface(), s.Len(), s.Cap())
	mm := reflect.MakeMap(reflect.TypeOf(map[Celsius]bool{}))
	mm.SetMapIndex(reflect.ValueOf(Celsius(1)), reflect.ValueOf(true))
	fmt.Println(mm.Interface())

	// Convert.
	cv := reflect.ValueOf(12.25).Convert(reflect.TypeOf(Celsius(0)))
	fmt.Println(cv.Type(), cv.Interface(), cv.Float())
	iv := reflect.ValueOf(Celsius(3.9)).Convert(reflect.TypeOf(0))
	fmt.Println(iv.Int(), reflect.ValueOf("x").CanConvert(reflect.TypeOf(0)), reflect.ValueOf(65).Convert(reflect.TypeOf("")).String())

	// Methods.
	ct := reflect.TypeOf(Celsius(0))
	pct := reflect.TypeOf(new(Celsius))
	fmt.Println(ct.NumMethod(), pct.NumMethod())
	for i := range pct.NumMethod() {
		fmt.Print(pct.Method(i).Name, " ")
	}
	fmt.Println()
	res := reflect.ValueOf(Celsius(1)).MethodByName("Add").Call([]reflect.Value{reflect.ValueOf(2.5)})
	fmt.Println(res[0].Interface(), res[0].Type())
	temp := Celsius(9)
	reflect.ValueOf(&temp).MethodByName("Reset").Call(nil)
	fmt.Println(temp, reflect.ValueOf(temp).MethodByName("Reset").IsValid())
	m, ok := ct.MethodByName("String")
	fmt.Println(m.Name, ok, m.Type)
	out := m.Func.Call([]reflect.Value{reflect.ValueOf(Celsius(2))})
	fmt.Println(out[0])

	// Implements and interface values.
	gt := reflect.TypeFor[Greeter]()
	st := reflect.TypeFor[fmt.Stringer]()
	fmt.Println(reflect.TypeOf(English{}).Implements(gt), ct.Implements(st), reflect.TypeOf(c).Implements(st), pct.Implements(st))
	var g Greeter = English{true}
	gv := reflect.ValueOf(&g).Elem()
	fmt.Println(gv.Kind(), gv.Elem().Type(), gv.Method(0).Call([]reflect.Value{reflect.ValueOf("vm")})[0])
	gv.Set(reflect.ValueOf(English{false}))
	fmt.Println(g.Greet("again"))

	// New, Zero, IsZero, DeepEqual.
	z := reflect.Zero(t).Interface().(Config)
	fmt.Println(z.Name == "", reflect.ValueOf(z).IsZero(), reflect.ValueOf(c).IsZero())
	np := reflect.New(reflect.TypeOf(English{}))
	np.Elem().Field(0).SetBool(true)
	fmt.Println(np.Interface().(*English).Greet("new"))
	a := map[string][]Celsius{"k": {1, 2}}
	b := map[string][]Celsius{"k": {1, 2}}
	fmt.Println(reflect.DeepEqual(a, b), reflect.DeepEqual(c, z), reflect.DeepEqual([]int(nil), []int{}))

	// Walk a struct generically.
	var walk func(prefix string, v reflect.Value)
	var lines []string
	walk = func(prefix string, v reflect.Value) {
		switch v.Kind() {
		case reflect.Struct:
			for i := range v.NumField() {
				if !v.Type().Field(i).IsExported() {
					continue
				}
				walk(prefix+"."+v.Type().Field(i).Name, v.Field(i))
			}
		case reflect.Pointer:
			if v.IsNil() {
				lines = append(lines, prefix+"=nil")
				return
			}
			walk(prefix, v.Elem())
		case reflect.Slice:
			lines = append(lines, fmt.Sprintf("%s=len%d", prefix, v.Len()))
		case reflect.Map:
			lines = append(lines, fmt.Sprintf("%s=map%d", prefix, v.Len()))
		default:
			lines = append(lines, fmt.Sprintf("%s=%v", prefix, v.Interface()))
		}
	}
	walk("c", reflect.ValueOf(&c))
	fmt.Println(strings.Join(lines, " "))

	// Func types.
	ft := reflect.TypeOf(strings.Repeat)
	fmt.Println(ft, ft.NumIn(), ft.In(1), ft.Out(0), ft.IsVariadic())
	fn := reflect.ValueOf(func(xs ...Celsius) Celsius {
		var s Celsius
		for _, x := range xs {
			s += x
		}
		return s
	})
	fmt.Println(fn.Call([]reflect.Value{reflect.ValueOf(Celsius(1)), reflect.ValueOf(Celsius(2))})[0])
	fmt.Println(fn.Type().IsVariadic(), fn.Type().In(0))

	// Chan and array types.
	cht := reflect.ChanOf(reflect.BothDir, ct)
	ch := reflect.MakeChan(cht, 1)
	ch.Send(reflect.ValueOf(Celsius(4)))
	rcv, ok := ch.Recv()
	fmt.Println(cht, rcv, ok)
	at := reflect.ArrayOf(3, ct)
	arr := reflect.New(at).Elem()
	arr.Index(2).SetFloat(8)
	fmt.Println(at, arr.Interface(), at.Len())
}
