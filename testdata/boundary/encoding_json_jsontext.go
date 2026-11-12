//go:build goexperiment.jsonv2

// Native boundary of encoding/json/jsontext: program types given to
// jsontext (docs/BOUNDARY.md). The package exists only with Go 1.27, or
// with GOEXPERIMENT=jsonv2.
package main

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"
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

// fmtTag is an option of the program: it embeds jsontext.Options (so it
// implements it) and has the ExperimentalSupportFormatTag marker method
// that jsonopts looks for.
type fmtTag struct {
	jsontext.Options
	on bool
}

func (o fmtTag) ExperimentalSupportFormatTag() bool { return o.on }

// pfmtTag has the marker method on the pointer.
type pfmtTag struct {
	jsontext.Options
	on bool
}

func (o *pfmtTag) ExperimentalSupportFormatTag() bool { return o.on }

// gfmtTag is a generic option with the marker method.
type gfmtTag[T any] struct {
	jsontext.Options
	v T
}

func (o gfmtTag[T]) ExperimentalSupportFormatTag() bool { return true }

// noTag implements jsontext.Options without the marker method.
type noTag struct{ jsontext.Options }

// Dur has a time.Duration: json/v2 has no default representation for it,
// and its error says more when the format tag is supported.
type Dur struct{ D time.Duration }

// options gives the options of the program to try.
func options() []jsontext.Options {
	return []jsontext.Options{fmtTag{on: true}, fmtTag{on: false}, &pfmtTag{on: true}, gfmtTag[int]{v: 1}, jsontext.Multiline(true)}
}

// try runs f and prints its panic, if any.
func try(label string, f func() string) {
	defer func() {
		if r := recover(); r != nil {
			out("%s: panic: %T: %v", label, r, r)
		}
	}()
	out("%s: %s", label, f())
}

func main() {
	// row: encoding/json/jsontext NewEncoder,(*Encoder).Reset,(*Encoder).WriteValue assert x.(encoding/json/internal/jsonopts.experimentalFormatTagSupporter)
	probe("Encoder/options", func() {
		for i, o := range options() {
			try(fmt.Sprint(i, " NewEncoder"), func() string {
				var buf bytes.Buffer
				enc := jsontext.NewEncoder(&buf, o)
				err := json.MarshalEncode(enc, Dur{})
				err2 := enc.WriteValue(jsontext.Value(`{"b":1,"a":[1,2]}`))
				return fmt.Sprintf("%v | %v | %q", err, err2, buf.String())
			})
			try(fmt.Sprint(i, " Reset"), func() string {
				var buf bytes.Buffer
				enc := jsontext.NewEncoder(&bytes.Buffer{})
				enc.Reset(&buf, o)
				err := enc.WriteValue(jsontext.Value(`{"b":1,"a":[1,2]}`))
				err2 := json.MarshalEncode(enc, Dur{})
				return fmt.Sprintf("%v | %v | %q", err, err2, buf.String())
			})
		}
		try("noTag NewEncoder", func() string {
			enc := jsontext.NewEncoder(&bytes.Buffer{}, noTag{})
			return fmt.Sprint(enc.WriteToken(jsontext.Null))
		})
	})
	// row: encoding/json/jsontext NewDecoder,(*Decoder).Reset assert x.(encoding/json/internal/jsonopts.experimentalFormatTagSupporter)
	probe("Decoder/options", func() {
		for i, o := range options() {
			try(fmt.Sprint(i, " NewDecoder"), func() string {
				dec := jsontext.NewDecoder(strings.NewReader(`{"D":1} {"x":2}`), o)
				var d Dur
				err := json.UnmarshalDecode(dec, &d)
				v, err2 := dec.ReadValue()
				return fmt.Sprintf("%v | %q %v", err, string(v), err2)
			})
			try(fmt.Sprint(i, " Reset"), func() string {
				dec := jsontext.NewDecoder(strings.NewReader(``))
				dec.Reset(strings.NewReader(`[1, 2] "s"`), o)
				v, err := dec.ReadValue()
				t, err2 := dec.ReadToken()
				return fmt.Sprintf("%q %v | %v %v", string(v), err, t, err2)
			})
		}
		try("noTag NewDecoder", func() string {
			dec := jsontext.NewDecoder(strings.NewReader(`1`), noTag{})
			v, err := dec.ReadValue()
			return fmt.Sprintf("%q %v", string(v), err)
		})
	})
	// row: encoding/json/jsontext Value.IsValid,(*Value).Canonicalize,(*Value).Compact,(*Value).Format,(*Value).Indent assert x.(encoding/json/internal/jsonopts.experimentalFormatTagSupporter)
	// row: encoding/json/jsontext (*Value).Canonicalize,(*Value).Compact,(*Value).Format,(*Value).Indent,(*Encoder).WriteValue assert x.(T), T unnamed
	// row: encoding/json/jsontext (*Value).Canonicalize,(*Value).Compact,(*Value).Format,(*Value).Indent,(*Encoder).WriteValue panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Value/options", func() {
		src := `{ "b" : 1.0, "a" : [ 2, {"d":1,"c":0} ], "e": "é" }`
		for i, o := range append(options(), noTag{}) {
			try(fmt.Sprint(i, " IsValid"), func() string { return fmt.Sprint(jsontext.Value(src).IsValid(o)) })
			try(fmt.Sprint(i, " Canonicalize"), func() string {
				v := jsontext.Value(src)
				err := v.Canonicalize(o)
				return fmt.Sprintf("%q %v", string(v), err)
			})
			try(fmt.Sprint(i, " Compact"), func() string {
				v := jsontext.Value(src)
				err := v.Compact(o)
				return fmt.Sprintf("%q %v", string(v), err)
			})
			try(fmt.Sprint(i, " Format"), func() string {
				v := jsontext.Value(src)
				err := v.Format(o, jsontext.CanonicalizeRawFloats(true))
				return fmt.Sprintf("%q %v", v, err)
			})
			try(fmt.Sprint(i, " Indent"), func() string {
				v := jsontext.Value(src)
				err := v.Indent(o)
				return fmt.Sprintf("%q %v", v, err)
			})
		}
		// the encoder sorts the members of objects with the deterministic option
		var buf bytes.Buffer
		enc := jsontext.NewEncoder(&buf, json.Deterministic(true), fmtTag{on: true})
		err := enc.WriteValue(jsontext.Value(`{"z":1,"y":{"b":2,"a":1}}`))
		out("WriteValue: %v %s", err, strings.TrimSpace(buf.String()))
	})
}
