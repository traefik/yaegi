// json.Unmarshal and json.Decoder.Decode into values whose type has methods.
// Before a fix, a pointer to a type with
// methods (pointer or value receiver) was not written back after the native
// call: the value stayed empty and the error was nil. The shapes come from
// plugins: negasus/traefik-plugin-bridge (a *Response with ToJSON, FromJSON
// and reset) and holysoles/bot-wrangler-traefik-plugin (a named map with a
// validate method).
package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// pointer methods, exported and unexported
type T struct{ Name string }

func (t *T) Reset() { *t = T{} }

type TU struct{ Name string }

func (t *TU) reset() { t.Name = "" }

// value methods
type TV struct{ Name string }

func (t TV) Get() string { return t.Name }

// shaped like the bridge Response
type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

func (r *Response) ToJSON() ([]byte, error) { return json.Marshal(r) }

func (r *Response) FromJSON(data []byte) error {
	r.reset()
	return json.Unmarshal(data, r)
}

func (r *Response) reset() {
	r.Status = 0
	r.Headers = nil
	r.Body = r.Body[:0]
}

// shaped like the bot-wrangler index
type Index map[string]int

func (m *Index) validate() error {
	if len(*m) == 0 {
		return errors.New("empty index")
	}
	return nil
}

func (m Index) keys() string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// named slice with a pointer method
type List []string

func (l *List) Add(s string) { *l = append(*l, s) }

// methods with hand-written native wrappers (String, Error)
type Str struct{ Name string }

func (s Str) String() string { return "Str(" + s.Name + ")" }

type StrP struct{ Name string }

func (s *StrP) String() string { return "StrP(" + s.Name + ")" }

type Err struct{ Code int }

func (e *Err) Error() string { return fmt.Sprint("code ", e.Code) }

type SMap map[string]int

func (m SMap) String() string { return "SMap:" + Index(m).keys() }

// UnmarshalJSON must be called
type Up struct{ Name string }

func (u *Up) UnmarshalJSON(b []byte) error {
	var raw struct{ Name string }
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	u.Name = strings.ToUpper(raw.Name)
	return nil
}

type UpMap map[string]string

func (m *UpMap) UnmarshalJSON(b []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*m = UpMap{}
	for k, v := range raw {
		(*m)[strings.ToUpper(k)] = v
	}
	return nil
}

// fields of types with methods. A field with an UnmarshalJSON method is not
// here: native json does not call the methods of nested interpreted values
// in default mode (MarshalJSON neither), a known limit.
type Outer struct {
	In  T
	P   *T
	Idx Index
	L   List
	S   Str
	E   Err
}

func main() {
	var t T
	err := json.Unmarshal([]byte(`{"Name":"n"}`), &t)
	fmt.Printf("T: %v %+v\n", err, t)

	var tu TU
	err = json.Unmarshal([]byte(`{"Name":"u"}`), &tu)
	fmt.Printf("TU: %v %+v\n", err, tu)

	var tv TV
	err = json.Unmarshal([]byte(`{"Name":"v"}`), &tv)
	fmt.Printf("TV: %v %q\n", err, tv.Get())

	r := &Response{Body: []byte("old")}
	err = r.FromJSON([]byte(`{"status":201,"headers":{"A":"1"},"body":"aGk="}`))
	fmt.Printf("Response: %v %d %v %s\n", err, r.Status, r.Headers, r.Body)
	b, err := r.ToJSON()
	fmt.Printf("ToJSON: %v %s\n", err, b)
	var r2 Response
	err = json.NewDecoder(strings.NewReader(string(b))).Decode(&r2)
	fmt.Printf("Decode Response: %v %d %v %s\n", err, r2.Status, r2.Headers, r2.Body)

	var idx Index
	err = json.Unmarshal([]byte(`{"bot":1,"crawler":2}`), &idx)
	fmt.Printf("Index nil: %v %v %v\n", err, idx.validate(), idx.keys())
	idx2 := Index{"old": 0}
	alias := idx2
	err = json.Unmarshal([]byte(`{"new":3}`), &idx2)
	fmt.Printf("Index non nil: %v %v %v alias %v\n", err, idx2.validate(), idx2.keys(), alias.keys())
	var idx3 Index
	err = json.NewDecoder(strings.NewReader(`{"d":4}`)).Decode(&idx3)
	fmt.Printf("Index Decode: %v %v\n", err, idx3.keys())
	var empty Index
	err = json.Unmarshal([]byte(`{}`), &empty)
	fmt.Printf("Index empty: %v %v\n", err, empty.validate())

	var l List
	err = json.Unmarshal([]byte(`["a","b"]`), &l)
	l.Add("c")
	fmt.Printf("List: %v %v\n", err, l)

	var s Str
	err = json.Unmarshal([]byte(`{"Name":"s"}`), &s)
	fmt.Printf("Str: %v %v\n", err, s)
	var sp StrP
	err = json.Unmarshal([]byte(`{"Name":"p"}`), &sp)
	fmt.Printf("StrP: %v %v\n", err, &sp)
	var e Err
	err = json.Unmarshal([]byte(`{"Code":7}`), &e)
	fmt.Printf("Err: %v %v\n", err, &e)
	var sm SMap
	err = json.Unmarshal([]byte(`{"x":1,"y":2}`), &sm)
	fmt.Printf("SMap: %v %v\n", err, sm)

	var u Up
	err = json.Unmarshal([]byte(`{"Name":"low"}`), &u)
	fmt.Printf("Up: %v %+v\n", err, u)
	var um UpMap
	err = json.NewDecoder(strings.NewReader(`{"k":"v"}`)).Decode(&um)
	fmt.Printf("UpMap: %v %v\n", err, um)
	err = json.Unmarshal([]byte(`{"Name":1}`), &u)
	fmt.Printf("Up error: %v %+v\n", err != nil, u)

	var o Outer
	err = json.Unmarshal([]byte(`{"In":{"Name":"i"},"P":{"Name":"p"},"Idx":{"z":9},"L":["x"],"S":{"Name":"s"},"E":{"Code":2}}`), &o)
	fmt.Printf("Outer: %v %+v %+v %v %v %v %v\n", err, o.In, *o.P, o.Idx.keys(), o.L, o.S, &o.E)

	// the decoded value through an interface and through a pointer kept elsewhere
	var any1 any = &T{}
	err = json.Unmarshal([]byte(`{"Name":"iface"}`), any1)
	fmt.Printf("iface: %v %+v\n", err, *any1.(*T))
	pt := &T{}
	keep := pt
	err = json.Unmarshal([]byte(`{"Name":"keep"}`), pt)
	fmt.Printf("keep: %v %+v\n", err, *keep)

	// errors are reported (their text names native types in default mode)
	var bad T
	err = json.Unmarshal([]byte(`{"Name":5}`), &bad)
	fmt.Printf("bad: %v %+v\n", err != nil, bad)
	var badIdx Index
	err = json.Unmarshal([]byte(`[1]`), &badIdx)
	fmt.Printf("bad Index: %v %v\n", err != nil, badIdx == nil)
	var badS Str
	err = json.Unmarshal([]byte(`{"Name":`), &badS)
	fmt.Printf("bad Str: %v %v\n", err, badS)

	// UnmarshalText of the destination, with a JSON string
	var tx Txt
	err = json.Unmarshal([]byte(`"a-b"`), &tx)
	fmt.Printf("Txt: %v %+v\n", err, tx)
	var tm TxtMap
	err = json.NewDecoder(strings.NewReader(`"k=v"`)).Decode(&tm)
	fmt.Printf("TxtMap: %v %v\n", err, tm)

	// other decoders: gob and xml with a String method
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(struct{ Name string }{"g"})
	var gs StrP
	err = gob.NewDecoder(&buf).Decode(&gs)
	fmt.Printf("gob StrP: %v %v\n", err, &gs)
	var xs XStr
	err = xml.Unmarshal([]byte(`<x><Name>x</Name></x>`), &xs)
	fmt.Printf("xml XStr: %v %v\n", err, xs)
	var xs2 XStr
	err = xml.NewDecoder(strings.NewReader(`<x><Name>y</Name></x>`)).Decode(&xs2)
	fmt.Printf("xml Decode XStr: %v %v\n", err, xs2)

	// a decoder behind an interface
	var dec interface{ Decode(any) error } = json.NewDecoder(strings.NewReader(`{"Name":"i"} {"a":1}`))
	var ds Str
	err = dec.Decode(&ds)
	fmt.Printf("iface Decode Str: %v %v\n", err, ds)
	var di Index
	err = dec.Decode(&di)
	fmt.Printf("iface Decode Index: %v %v\n", err, di.keys())

	// a nil pointer whose type has UnmarshalJSON or UnmarshalText: json
	// returns an InvalidUnmarshalError and does not call the method
	var nu *Up
	err = json.Unmarshal([]byte(`{"Name":"nil"}`), nu)
	var ie *json.InvalidUnmarshalError
	fmt.Printf("nil Up: %v %v\n", errors.As(err, &ie), nu == nil)
	var ntx *Txt
	err = json.Unmarshal([]byte(`"a-b"`), ntx)
	fmt.Printf("nil Txt: %v %v\n", errors.As(err, &ie), ntx == nil)
	var nb *Both
	err = json.NewDecoder(strings.NewReader(`"x"`)).Decode(nb)
	fmt.Printf("nil Both: %v %v\n", errors.As(err, &ie), nb == nil)

	var mj Mj
	err = json.Unmarshal([]byte(`{"N":3}`), &mj)
	b, _ = json.Marshal(mj)
	fmt.Printf("Mj: %v %d %s\n", err, mj.N, b)
}

type Txt struct{ A, B string }

func (t *Txt) UnmarshalText(b []byte) error {
	t.A, t.B, _ = strings.Cut(string(b), "-")
	return nil
}

type TxtMap map[string]string

func (m *TxtMap) UnmarshalText(b []byte) error {
	k, v, _ := strings.Cut(string(b), "=")
	*m = TxtMap{k: v}
	return nil
}

// both UnmarshalJSON and UnmarshalText
type Both struct{ S string }

func (b *Both) UnmarshalJSON(d []byte) error { b.S = "json:" + string(d); return nil }

func (b *Both) UnmarshalText(d []byte) error { b.S = "text:" + string(d); return nil }

// MarshalJSON only: decoding fills the fields
type Mj struct{ N int }

func (m Mj) MarshalJSON() ([]byte, error) { return []byte(`{"N":99}`), nil }

type XStr struct {
	XMLName xml.Name `xml:"x"`
	Name    string
}

func (s XStr) String() string { return "XStr(" + s.Name + ")" }
