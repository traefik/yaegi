//go:build goexperiment.jsonv2

package main

// The generic functions of encoding/json/v2 (Go 1.27) run for the types of
// the program: GetOption, MarshalFunc, MarshalToFunc,
// UnmarshalFunc and UnmarshalFromFunc. The VM refused them at load time.
// A pointer that the function gets is the pointer of the program: a write
// through it reaches the variable, also at the top level (the copy of
// json/v2 sees a native shadow of a *string there), for an element of an
// array and for the value that json/v2 allocates for a nil pointer.
// GetOption also takes json.WithMarshalers, a function of the copy.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type celsius float64

type point struct {
	X, Y int
}

type box struct {
	Name  string
	Temp  celsius
	At    point
	Ptr   *point
	Many  []celsius
	Codes map[string]point
}

func main() {
	// GetOption
	opts := json.JoinOptions(json.StringifyNumbers(true), jsontext.WithIndent("\t"))
	v, ok := json.GetOption(opts, json.StringifyNumbers)
	fmt.Println("StringifyNumbers:", v, ok)
	v, ok = json.GetOption(opts, json.Deterministic)
	fmt.Println("Deterministic:", v, ok)
	ind, ok := json.GetOption(opts, jsontext.WithIndent)
	fmt.Printf("WithIndent: %q %v\n", ind, ok)
	ind, ok = json.GetOption(nil, jsontext.WithIndent)
	fmt.Printf("nil: %q %v\n", ind, ok)

	// MarshalFunc and MarshalToFunc for types of the program
	m := json.JoinMarshalers(
		json.MarshalFunc(func(c celsius) ([]byte, error) {
			return fmt.Appendf(nil, `"%.1fC"`, float64(c)), nil
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, p point) error {
			return enc.WriteToken(jsontext.String(fmt.Sprintf("%d:%d", p.X, p.Y)))
		}),
		json.MarshalFunc(func(d time.Duration) ([]byte, error) {
			return []byte(`"` + d.String() + `"`), nil
		}),
	)
	b := box{Name: "a", Temp: 21.5, At: point{1, 2}, Ptr: &point{3, 4}, Many: []celsius{1, 2}, Codes: map[string]point{"k": {5, 6}}}
	out, err := json.Marshal(b, json.WithMarshalers(m), json.Deterministic(true))
	fmt.Println(string(out), err)
	out, err = json.Marshal([]any{time.Second, celsius(3), point{7, 8}}, json.WithMarshalers(m))
	fmt.Println(string(out), err)

	// errors.ErrUnsupported skips the function, another error stops
	skip := json.MarshalToFunc(func(enc *jsontext.Encoder, c celsius) error {
		if c < 0 {
			return errors.New("too cold")
		}
		return errors.ErrUnsupported
	})
	out, err = json.Marshal([]celsius{1, 2}, json.WithMarshalers(skip))
	fmt.Println(string(out), err)
	_, err = json.Marshal([]celsius{-1}, json.WithMarshalers(skip))
	fmt.Println(err != nil, err != nil && strings.Contains(err.Error(), "too cold"))

	// UnmarshalFunc and UnmarshalFromFunc
	u := json.JoinUnmarshalers(
		json.UnmarshalFunc(func(data []byte, c *celsius) error {
			var s string
			if err := json.Unmarshal(data, &s); err != nil {
				return err
			}
			var f float64
			_, err := fmt.Sscanf(strings.TrimSuffix(s, "C"), "%g", &f)
			*c = celsius(f)
			return err
		}),
		json.UnmarshalFromFunc(func(dec *jsontext.Decoder, p *point) error {
			tok, err := dec.ReadToken()
			if err != nil {
				return err
			}
			_, err = fmt.Sscanf(tok.String(), "%d:%d", &p.X, &p.Y)
			return err
		}),
	)
	var got box
	err = json.Unmarshal(out0(b, m), &got, json.WithUnmarshalers(u))
	fmt.Printf("%+v %+v %v\n", got.At, *got.Ptr, err)
	fmt.Println(got.Name, got.Temp, got.Many, got.Codes, err)

	// an interface value: the function of an interface type gets it
	ui := json.UnmarshalFunc(func(data []byte, v *any) error {
		*v = "any:" + string(data)
		return nil
	})
	var anyv any
	err = json.Unmarshal([]byte(`[1,"x"]`), &anyv, json.WithUnmarshalers(ui))
	fmt.Println(anyv, err)

	// the values of native types, at the top level and in an any
	md := json.MarshalFunc(func(d time.Duration) ([]byte, error) {
		return []byte(`"` + d.String() + `"`), nil
	})
	out, err = json.Marshal(time.Second, json.WithMarshalers(md))
	fmt.Println("top", string(out), err)
	out, err = json.Marshal(struct{ A any }{time.Minute}, json.WithMarshalers(md))
	fmt.Println("any field", string(out), err)

	// the function writes through the pointer of the program
	var s string
	us := json.UnmarshalFunc(func(data []byte, v *string) error {
		fmt.Println("same pointer:", v == &s)
		*v = "s:" + string(data)
		return nil
	})
	err = json.Unmarshal([]byte(`1`), &s, json.WithUnmarshalers(us))
	fmt.Println("top string", s, err)
	ps := &s
	err = json.Unmarshal([]byte(`2`), &ps, json.WithUnmarshalers(us))
	fmt.Println("pointer to pointer", s, err)
	var wrap struct{ A any }
	err = json.Unmarshal([]byte(`{"A":[1]}`), &wrap, json.WithUnmarshalers(ui))
	fmt.Println("any field", wrap.A, err)

	// elements of an array and nil pointers that the copy allocates: the
	// value is in the memory of a shadow (callWithShadows)
	ui2 := json.UnmarshalFunc(func(data []byte, v *int) error {
		n, err := strconv.Atoi(string(data))
		*v = n * 100
		return err
	})
	us2 := json.UnmarshalFunc(func(data []byte, v *string) error {
		*v = "s:" + string(data)
		return nil
	})
	var ai [2]int
	err = json.Unmarshal([]byte(`[1,2]`), &ai, json.WithUnmarshalers(ui2))
	fmt.Println("array int", ai, err)
	var as [2]string
	err = json.Unmarshal([]byte(`["a","b"]`), &as, json.WithUnmarshalers(us2))
	fmt.Println("array string", as, err)
	var pi *int
	err = json.Unmarshal([]byte(`3`), &pi, json.WithUnmarshalers(ui2))
	fmt.Println("nil pointer", pi != nil && *pi == 300, err)
	var pstr *string
	err = json.Unmarshal([]byte(`"x"`), &pstr, json.WithUnmarshalers(us2))
	fmt.Println("nil string pointer", pstr != nil && *pstr == `s:"x"`, err)
	uf := json.UnmarshalFromFunc(func(dec *jsontext.Decoder, v *int) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(tok.String())
		*v = n + 1
		return err
	})
	var af [3]int
	err = json.Unmarshal([]byte(`[1,2,3]`), &af, json.WithUnmarshalers(uf))
	fmt.Println("array from", af, err)

	// GetOption of WithMarshalers and WithUnmarshalers
	mi := json.MarshalFunc(func(v int) ([]byte, error) { return []byte(`"i"`), nil })
	opts = json.JoinOptions(json.WithMarshalers(mi), json.Deterministic(true))
	gm, ok := json.GetOption(opts, json.WithMarshalers)
	fmt.Println("GetOption WithMarshalers", gm == mi, ok)
	_, ok = json.GetOption(opts, json.WithUnmarshalers)
	fmt.Println("GetOption WithUnmarshalers", ok)
}

func out0(b box, m *json.Marshalers) []byte {
	out, err := json.Marshal(b, json.WithMarshalers(m))
	if err != nil {
		panic(err)
	}
	return out
}
