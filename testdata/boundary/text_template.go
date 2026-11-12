// Native boundary of text/template: program types given to text/template
// (docs/BOUNDARY.md).
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"
	"text/template"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// show prints a text line by line.
func show(title, text string) {
	for _, l := range strings.Split(text, "\n") {
		out("%s| %s", title, l)
	}
}

// Name is a named string type.
type Name string

// Count is a named int type.
type Count int

// Flag is a named bool type.
type Flag bool

// Celsius is a named float type with a String method.
type Celsius float64

func (c Celsius) String() string { return fmt.Sprintf("%.1f°C", float64(c)) }

// Point has a String method with a pointer receiver.
type Point struct{ X, Y int }

func (p *Point) String() string { return fmt.Sprintf("P(%d,%d)", p.X, p.Y) }

// Key is a struct map key.
type Key struct {
	A string
	B int
}

// Inner is embedded in User.
type Inner struct{ Zone string }

func (i Inner) Where() string { return "zone " + i.Zone }

// Err is a program error type.
type Err struct{ Code int }

func (e *Err) Error() string { return fmt.Sprintf("err %d", e.Code) }

// Box is comparable, but its field may hold an uncomparable value.
type Box struct{ V any }

// Op is a named func type.
type Op func(int, int) int

// Pair is a generic type.
type Pair[K comparable, V any] struct {
	K K
	V V
}

func (p Pair[K, V]) Swap() string { return fmt.Sprintf("%v=%v", p.V, p.K) }

// User is the main data of the templates.
type User struct {
	Inner
	Name     Name
	Age      Count
	Admin    Flag
	Temp     Celsius
	Pos      Point
	PPos     *Point
	Tags     []Name
	Scores   map[Name]Count
	ByKey    map[Key]string
	ByCount  map[Count]string
	Any      any
	AnyMap   map[string]any
	Nil      *User
	Ch       chan Count
	Add      Op
	Pair     Pair[string, Count]
	Arr      [2]Count
	Seq      iter.Seq[Count]
	Seq2     iter.Seq2[Name, Count]
	Bytes    []byte
	Cx       complex128
	U        uint8
	private  string
	Boxes    []Box
	ErrField error
}

func (u User) Greet(greeting string) string { return string(greeting) + ", " + string(u.Name) }
func (u *User) Older(n Count) Count         { return u.Age + n }
func (u User) Check(ok bool) (string, error) {
	if !ok {
		return "", &Err{42}
	}
	return "checked", nil
}
func (u User) Join(sep string, parts ...Name) string {
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = string(p)
	}
	return strings.Join(s, sep)
}
func (u User) Boom(kind string) string {
	switch kind {
	case "error":
		panic(&Err{7})
	case "value":
		panic(Key{"k", 1})
	}
	return "no boom"
}
func (u User) Self() User    { return u }
func (u User) IsAdult() Flag { return u.Age >= 18 }

func newUser() *User {
	ch := make(chan Count, 3)
	ch <- 1
	ch <- 2
	close(ch)
	return &User{
		Inner:   Inner{"eu"},
		Name:    "ann",
		Age:     30,
		Admin:   true,
		Temp:    21.5,
		Pos:     Point{1, 2},
		PPos:    &Point{3, 4},
		Tags:    []Name{"a", "b"},
		Scores:  map[Name]Count{"z": 1, "m": 2, "a": 3},
		ByKey:   map[Key]string{{"b", 1}: "b1", {"a", 2}: "a2", {"a", 1}: "a1"},
		ByCount: map[Count]string{3: "three", -1: "minus", 2: "two"},
		Any:     Pair[string, Count]{"k", 9},
		AnyMap:  map[string]any{"n": Count(5), "p": &Point{5, 6}, "s": Name("x"), "f": Celsius(1)},
		Ch:      ch,
		Add:     func(a, b int) int { return a + b },
		Pair:    Pair[string, Count]{"x", 1},
		Arr:     [2]Count{7, 8},
		Seq: func(yield func(Count) bool) {
			for i := Count(0); i < 3; i++ {
				if !yield(i * 10) {
					return
				}
			}
		},
		Seq2: func(yield func(Name, Count) bool) {
			_ = yield("one", 1) && yield("two", 2)
		},
		Bytes:    []byte("hi"),
		Cx:       1 + 2i,
		U:        200,
		private:  "secret",
		Boxes:    []Box{{1}, {"s"}},
		ErrField: &Err{3},
	}
}

// main template: fields, methods, maps, slices, channels, iterators,
// comparisons and the builtins, on program types.
const mainText = `{{define "row"}}[{{.}}]{{end -}}
name={{.Name}} age={{.Age}} admin={{.Admin}} temp={{.Temp}} zone={{.Zone}} where={{.Where}}
pos={{.Pos}} ppos={{.PPos}} &pos={{.Pos.X}},{{.Pos.Y}} any={{.Any}} anyK={{.Any.K}} swap={{.Any.Swap}} pair={{.Pair.Swap}}
greet={{.Greet "hello"}} older={{.Older 5}} join={{.Join "-" "x" "y"}} self={{.Self.Name}} adult={{.IsAdult}}
check={{.Check true}} arr={{.Arr}} arr0={{index .Arr 0}} bytes={{.Bytes}} printf={{printf "%v|%d|%s" .Temp .Age .Name}}
cx={{.Cx}} u={{.U}} len={{len .Tags}} {{len .Scores}} {{len .Arr}} slice={{slice .Tags 1}} idx={{index .AnyMap "n"}} {{index .Tags 0}} {{index .Arr 1}}
tags={{range $i, $t := .Tags}}{{$i}}:{{$t}} {{end}}
scores={{range $k, $v := .Scores}}{{$k}}={{$v}} {{end}}
bykey={{range $k, $v := .ByKey}}{{$k.A}}{{$k.B}}={{$v}} {{end}}
bycount={{range $k, $v := .ByCount}}{{$k}}={{$v}} {{end}}
anymap={{range $k, $v := .AnyMap}}{{$k}}={{$v}} {{end}}
chan={{range .Ch}}{{.}} {{end}}
int={{range 3}}{{.}}{{end}} call={{call .Add 2 3}}
if={{if .Admin}}admin{{end}} {{if .Nil}}nil?{{else}}no nil{{end}} {{if .Tags}}tags{{end}} {{with .Any}}with {{.K}}{{end}}
cmp={{eq .Age 30}} {{eq .Name "ann"}} {{ne .Admin false}} {{lt .Age 40}} {{le .Temp 21.5}} {{gt .U 100}} {{eq .Age 30 31}} {{eq .Pair .Pair}}
andor={{and .Admin .Age}} {{or .Nil .Name}} {{.Age | and .Admin}} {{.Name | or .Nil}} {{not .Admin}}
pipe={{.Age | printf "%03d"}} {{.Name | printf "%q"}} html={{html .Name}} js={{js .Name}} url={{urlquery .Name "a b"}} print={{print .Pos .PPos}}
tmpl={{template "row" .Age}} {{template "row" .Tags}}
errfield={{.ErrField}} boxes={{range .Boxes}}{{.V}} {{end}}`

func main() {
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate reflect *
	probe("Execute", func() {
		t := template.Must(template.New("main").Parse(mainText))
		var sb strings.Builder
		err := t.Execute(&sb, newUser())
		show("ptr", sb.String())
		out("err: %v", err)
		sb.Reset()
		// methods with a pointer receiver are not in the method set of User
		t = template.Must(template.New("main").Parse(strings.Replace(mainText, " older={{.Older 5}}", "", 1)))
		err = t.ExecuteTemplate(&sb, "main", *newUser())
		show("value", sb.String())
		out("err: %v", err)
		sb.Reset()
		var data any = map[string]any{"Name": Name("m"), "Age": Count(1)}
		err = template.Must(template.New("m").Parse(`{{.Name}} {{.Age}} {{.Missing}}`)).Execute(&sb, data)
		out("map: %q %v", sb.String(), err)
		sb.Reset()
		err = template.Must(template.New("s").Parse(`{{range .}}{{.K}}={{.V}};{{end}}`)).Execute(&sb, []Pair[Name, Count]{{"a", 1}, {"b", 2}})
		out("generic slice: %q %v", sb.String(), err)
	})
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate reflect walk
	probe("Execute/seq", func() {
		t := template.Must(template.New("seq").Parse(`seq={{range .Seq}}{{.}} {{end}}`))
		var sb strings.Builder
		err := t.Execute(&sb, newUser())
		out("%q %v", sb.String(), err)
		sb.Reset()
		t = template.Must(template.New("seq2").Parse(`seq2={{range $k, $v := .Seq2}}{{$k}}:{{$v}} {{end}}`))
		err = t.Execute(&sb, newUser())
		out("%q %v", sb.String(), err)
	})
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate reflect *
	probe("Execute/errors", func() {
		var sb strings.Builder
		err := template.Must(template.New("v").Parse(`{{.Older 1}}`)).Execute(&sb, *newUser())
		out("value Older: %q %v", sb.String(), err)
		for _, src := range []string{
			`{{.Private}}`,
			`{{.private}}`,
			`{{.Name.Foo}}`,
			`{{.Older "x"}}`,
			`{{.Greet}}`,
			`{{index .Tags 5}}`,
			`{{index .Scores 1}}`,
			`{{len .Age}}`,
			`{{slice .Arr 1}}`,
			`{{call .Name}}`,
			`{{eq .Age "x"}}`,
			`{{lt .Pair .Pair}}`,
			`{{range .Age}}{{end}}`,
			`{{.Nil.Name}}`,
			`{{template "nope"}}`,
		} {
			var sb strings.Builder
			t, err := template.New("e").Parse(src)
			if err == nil {
				err = t.Execute(&sb, newUser())
			}
			out("%s -> %q %v", src, sb.String(), err)
		}
	})
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate assert x.(error)
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate panic x.(error) without ok: *runtime.TypeAssertionError
	probe("Execute/safeCall", func() {
		for _, src := range []string{
			`{{.Check true}}`,
			`{{.Check false}}`,
			`{{.Boom "error"}}`,
			`{{.Boom "value"}}`,
			`{{.Boom "none"}}`,
			`{{fail 1}}`,
			`{{fail 0}}`,
			`{{bad}}`,
		} {
			var sb strings.Builder
			t := template.Must(template.New("c").Funcs(template.FuncMap{
				"fail": func(n Count) (Name, error) {
					if n == 0 {
						return "", &Err{int(n)}
					}
					return Name(fmt.Sprint("ok", n)), nil
				},
				"bad": func() (int, error) { panic(errors.New("bad func")) },
			}).Parse(src))
			err := t.Execute(&sb, newUser())
			var e *Err
			out("%s -> %q %v; As *Err: %v", src, sb.String(), err, errors.As(err, &e))
		}
	})
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate panic panic message with the type name
	probe("Execute/sortkeys", func() {
		data := map[string]any{
			"ByKey":   map[Key]int{{"b", 2}: 1, {"a", 9}: 2, {"a", 1}: 3},
			"ByPtr":   map[*Key]int{{"x", 1}: 1},
			"ByIface": map[any]int{Key{"b", 1}: 1, Key{"a", 1}: 2},
			"ByBox":   map[Box]int{{Count(2)}: 1, {Count(1)}: 2},
			"ByArr":   map[[2]Name]int{{"b", "a"}: 1, {"a", "z"}: 2},
			"ByFlag":  map[Flag]int{true: 1, false: 2},
			"ByCx":    map[complex64]int{2i: 1, 1: 2},
		}
		t := template.Must(template.New("k").Parse(
			`{{range $k, $v := .ByKey}}{{$k}}={{$v}} {{end}}|{{range $k, $v := .ByPtr}}{{$k.A}}={{$v}} {{end}}|{{range $k, $v := .ByIface}}{{$k}}={{$v}} {{end}}|{{range $k, $v := .ByBox}}{{$k}}={{$v}} {{end}}|{{range $k, $v := .ByArr}}{{$k}}={{$v}} {{end}}|{{range $k, $v := .ByFlag}}{{$k}}={{$v}} {{end}}|{{range $k, $v := .ByCx}}{{$k}}={{$v}} {{end}}`))
		var sb strings.Builder
		err := t.Execute(&sb, data)
		out("%s %v", sb.String(), err)
	})
	// row: text/template (*Template).Execute,(*Template).ExecuteTemplate panic == of two interfaces: comparing uncomparable type
	probe("Execute/uncomparable", func() {
		for _, src := range []string{
			`{{eq (index .Boxes 0) (index .Boxes 0)}}`,
			`{{eq .Slice .Slice}}`,
			`{{eq .A .B}}`,
			`{{eq .A .C}}`,
			`{{eq .Tags .Tags}}`,
			`{{eq .Pair .Pair}}`,
			`{{.A | and .B}}`,
			`{{.C | or .Nil}}`,
			`{{.A | eq .A}}`,
		} {
			data := map[string]any{
				"Boxes": []Box{{[]int{1}}},
				"Slice": []Name{"x"},
				"A":     Box{[]int{1}},
				"B":     Box{[]int{1}},
				"C":     Box{1},
				"Tags":  []Name{"a"},
				"Pair":  Pair[string, any]{"k", []int{1}},
				"Nil":   (*User)(nil),
			}
			var sb strings.Builder
			err := template.Must(template.New("u").Parse(src)).Execute(&sb, data)
			out("%s -> %q %v", src, sb.String(), err)
		}
	})
	// row: text/template (*Template).Funcs reflect *
	probe("Funcs", func() {
		try := func(name string, fm template.FuncMap) {
			defer func() {
				if r := recover(); r != nil {
					out("%s: panic %T: %v", name, r, r)
				}
			}()
			t := template.Must(template.New(name).Funcs(fm).Parse(`{{f 2 3}}`))
			var sb strings.Builder
			err := t.Execute(&sb, nil)
			out("%s: %q %v", name, sb.String(), err)
		}
		try("Op", template.FuncMap{"f": Op(func(a, b int) int { return a * b })})
		try("func", template.FuncMap{"f": func(a, b Count) Name { return Name(fmt.Sprint(a + b)) }})
		try("generic", template.FuncMap{"f": Pair[int, int].Swap})
		try("method value", template.FuncMap{"f": newUser().Join})
		try("not func", template.FuncMap{"f": Count(1)})
		try("3 results", template.FuncMap{"f": func(a, b int) (int, int, error) { return 0, 0, nil }})
		try("bad 2nd", template.FuncMap{"f": func(a, b int) (int, *Err) { return 0, nil }})
		try("no result", template.FuncMap{"f": func(a, b int) {}})
	})
	// row: text/template (*Template).Parse,(*Template).ParseFS,(*Template).ParseFiles,(*Template).ParseGlob,ParseFS,ParseFiles,ParseGlob * *
	probe("Parse", func() {
		const src = `{{eq .Age 30}} {{eq .Name "ann"}} {{eq .Pair .Pair}} {{eq .Admin true}} {{lt .Age 31}} {{ge .Temp 1.5}} {{eq .U 200}} {{eq .Cx .Cx}}` +
			` {{index .Tags 1}} {{index .Arr 1}} {{index .AnyMap "s"}} {{slice .Tags 0 1}} {{slice .Bytes 1}} {{slice .Tags 0 1 2}} {{len .Ch}}` +
			` {{html .Name}} {{js .Temp}} {{urlquery .PPos}} {{print .Pos}} {{print .PPos}} {{println .Temp}}{{printf "%v" .Any}}` +
			` {{eq (index .Boxes 0) (index .Boxes 0)}}`
		dir, err := os.MkdirTemp("", "boundary-tt")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(dir)
		os.WriteFile(filepath.Join(dir, "a.tmpl"), []byte(src), 0o644)
		mfs := fstest.MapFS{"a.tmpl": &fstest.MapFile{Data: []byte(src)}}
		u := newUser()
		u.Boxes = []Box{{[]int{1}}}
		run := func(how string, t *template.Template, err error) {
			if err != nil {
				out("%s: %v", how, err)
				return
			}
			var sb strings.Builder
			err = t.ExecuteTemplate(&sb, "a.tmpl", u)
			out("%s: %q %v", how, sb.String(), err)
		}
		t, err := template.New("a.tmpl").Parse(src)
		run("Parse", t, err)
		t, err = template.ParseFiles(filepath.Join(dir, "a.tmpl"))
		run("ParseFiles", t, err)
		t, err = template.ParseGlob(filepath.Join(dir, "*.tmpl"))
		run("ParseGlob", t, err)
		t, err = template.ParseFS(mfs, "*.tmpl")
		run("ParseFS", t, err)
		t, err = template.ParseFS(progFS{mfs}, "*.tmpl")
		run("ParseFS program", t, err)
		t, err = template.New("x").ParseFiles(filepath.Join(dir, "a.tmpl"))
		run("T.ParseFiles", t, err)
		t, err = template.New("x").ParseGlob(filepath.Join(dir, "*.tmpl"))
		run("T.ParseGlob", t, err)
		t, err = template.New("x").ParseFS(progFS{mfs}, "a.tmpl")
		run("T.ParseFS", t, err)
	})
	// row: text/template HTMLEscaper,JSEscaper,URLQueryEscaper * *
	probe("Escapers", func() {
		p := &Point{1, 2}
		for _, args := range [][]any{
			{Name("<a&b>")},
			{"<plain>"},
			{Count(3), Flag(true)},
			{Celsius(2)},
			{Point{1, 2}},
			{p},
			{&Err{5}},
			{Pair[Name, Count]{"<k>", 1}},
			{[]Name{"x", "y"}},
			{(*Point)(nil)},
			{nil},
		} {
			out("%v: html=%q js=%q url=%q", len(args), template.HTMLEscaper(args...), template.JSEscaper(args...), template.URLQueryEscaper(args...))
		}
	})
	// row: text/template IsTrue reflect *
	probe("IsTrue", func() {
		var nilp *User
		var nilOp Op
		var nilCh chan Count
		for _, v := range []any{
			Flag(true), Flag(false), Count(0), Count(2), Name(""), Name("x"), Celsius(0), Celsius(1),
			User{}, nilp, &User{}, []Name{}, []Name{"a"}, map[Name]Count{}, map[Name]Count{"a": 1},
			[0]Count{}, [1]Count{}, nilOp, Op(func(a, b int) int { return 0 }), nilCh, make(chan Count),
			complex64(0), uint8(0), uint8(1), Pair[int, int]{}, nil, any(nil), error(nil), (*Err)(nil),
		} {
			truth, ok := template.IsTrue(v)
			out("%T: %v %v", v, truth, ok)
		}
	})
}

// progFS is a program fs.FS over a MapFS.
type progFS struct{ m fstest.MapFS }

func (p progFS) Open(name string) (fs.File, error) { return p.m.Open(name) }
