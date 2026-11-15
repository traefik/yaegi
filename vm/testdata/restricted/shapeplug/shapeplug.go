// Package shapeplug copies a native struct with unexported fields into a
// struct of its own of the same shape, with reflect.Value.Set and
// Value.Convert: Go refuses both (the unexported fields of two packages
// are never the same fields), so the plugin must not read the fields of a
// host object that way.
package shapeplug

import (
	"fmt"
	"image"
	"reflect"
	"strings"
)

// reader has the shape of strings.Reader.
type reader struct {
	s        string
	i        int64
	prevRune int
}

// Copy returns the text of r by Set, by Convert and by Set of the pointer,
// or the panic messages.
func Copy() (set, convert, pointer string) {
	src := reflect.ValueOf(strings.NewReader("secret")).Elem()
	set = try(func() string {
		var mine reader
		reflect.ValueOf(&mine).Elem().Set(src)
		return mine.s
	})
	pointer = try(func() string {
		var mine *reader
		reflect.ValueOf(&mine).Elem().Set(src.Addr())
		return mine.s
	})
	convert = try(func() string {
		return src.Convert(reflect.TypeOf(reader{})).Interface().(reader).s
	})
	return
}

// point has the fields of image.Point; tagged too, with a struct tag.
type point struct{ X, Y int }

type tagged struct {
	X int `json:"x"`
	Y int
}

// Exported converts an image.Point, whose fields are all exported, to
// point, to tagged and by pointer to *point: Go allows the three (the
// names and the tags do not count in a conversion).
func Exported() (value, tag, pointer string) {
	src := reflect.ValueOf(image.Point{X: 1, Y: 2})
	value = try(func() string {
		return fmt.Sprint(src.Convert(reflect.TypeOf(point{})).Interface().(point))
	})
	tag = try(func() string {
		return fmt.Sprint(src.Convert(reflect.TypeOf(tagged{})).Interface().(tagged))
	})
	pointer = try(func() string {
		p := reflect.ValueOf(&image.Point{X: 1, Y: 2}).Convert(reflect.TypeOf(&point{}))
		return fmt.Sprint(p.Elem().Field(0).Int(), p.Elem().Field(1).Int())
	})
	return
}

// Stores copies the strings.Reader into a reader with the other stores of
// reflect: Append, Copy, SetMapIndex (value and key), Send and the send of
// Select. Go refuses all of them.
func Stores() []string {
	src := reflect.ValueOf(strings.NewReader("secret")).Elem()
	return []string{
		try(func() string {
			return reflect.Append(reflect.ValueOf([]reader{}), src).Index(0).Interface().(reader).s
		}),
		try(func() string {
			dst := make([]reader, 1)
			reflect.Copy(reflect.ValueOf(dst), reflect.ValueOf([]strings.Reader{*strings.NewReader("secret")}))
			return dst[0].s
		}),
		try(func() string {
			m := map[string]reader{}
			reflect.ValueOf(m).SetMapIndex(reflect.ValueOf("k"), src)
			return m["k"].s
		}),
		try(func() string {
			ch := make(chan reader, 1)
			reflect.ValueOf(ch).Send(src)
			return (<-ch).s
		}),
		try(func() string {
			m := map[reader]int{}
			reflect.ValueOf(m).SetMapIndex(src, reflect.ValueOf(1))
			for k := range m {
				return k.s
			}
			return ""
		}),
		try(func() string {
			ch := make(chan reader, 1)
			reflect.Select([]reflect.SelectCase{{Dir: reflect.SelectSend, Chan: reflect.ValueOf(ch), Send: src}})
			return (<-ch).s
		}),
	}
}

func try(f func() string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = "panic: " + strings.SplitN(fmtAny(r), "\n", 2)[0]
		}
	}()
	return f()
}

func fmtAny(r any) string {
	if e, ok := r.(error); ok {
		return e.Error()
	}
	if s, ok := r.(string); ok {
		return s
	}
	return "?"
}
