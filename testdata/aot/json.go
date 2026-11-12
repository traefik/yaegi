// AOT test program: encoding/json on interpreted types.
//
//aot:lib encoding/json
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Level is a named non-struct type with String and text marshaling.
type Level int

func (l Level) String() string { return [...]string{"debug", "info", "warn", "error"}[l] }

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	for i, s := range [...]string{"debug", "info", "warn", "error"} {
		if s == string(b) {
			*l = Level(i)
			return nil
		}
	}
	return fmt.Errorf("bad level %q", b)
}

// Stamp has a value-receiver MarshalJSON and a pointer-receiver UnmarshalJSON.
type Stamp struct {
	Sec  int64
	Zone string
}

func (s Stamp) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(fmt.Sprintf("%d@%s", s.Sec, s.Zone))), nil
}

func (s *Stamp) UnmarshalJSON(b []byte) error {
	str, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	sec, zone, ok := strings.Cut(str, "@")
	if !ok {
		return errors.New("stamp: missing @")
	}
	s.Sec, err = strconv.ParseInt(sec, 10, 64)
	s.Zone = zone
	return err
}

type Celsius float64

type Tags []string

type Base struct {
	ID      int    `json:"id"`
	Created string `json:"created,omitempty"`
}

type Item struct {
	Name  string  `json:"name"`
	Price Celsius `json:"price"`
	Qty   int     `json:"qty,omitempty"`
	Note  *string `json:"note,omitempty"`
}

type Payload struct {
	Base
	User    string            `json:"user"`
	Level   Level             `json:"level"`
	When    Stamp             `json:"when"`
	Items   []Item            `json:"items"`
	Tags    Tags              `json:"tags"`
	Meta    map[string]string `json:"meta"`
	ByLevel map[Level]int     `json:"by_level"`
	Extra   any               `json:"extra"`
	Count   int64             `json:"count,string"`
	Raw     json.RawMessage   `json:"raw"`
	Num     json.Number       `json:"num"`
	Ptr     *Item             `json:"ptr"`
	skip    int
	Dur     time.Duration `json:"dur"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	note := "fragile"
	p := Payload{
		Base:    Base{ID: 7},
		User:    "alice",
		Level:   2,
		When:    Stamp{1700000000, "UTC"},
		Items:   []Item{{Name: "kbd", Price: 49.5, Qty: 1}, {Name: "glass", Price: 3, Note: &note}},
		Tags:    Tags{"a", "b"},
		Meta:    map[string]string{"z": "1", "a": "2"},
		ByLevel: map[Level]int{1: 10, 3: 30},
		Extra:   map[string]any{"k": []any{1, "two", 3.5, nil, true}},
		Count:   42,
		Raw:     json.RawMessage(`{"pre":"computed"}`),
		Num:     "12.50",
		skip:    1,
		Dur:     time.Second,
	}
	b, err := json.Marshal(p)
	fmt.Println(string(b), err)

	var q Payload
	err = json.Unmarshal(b, &q)
	fmt.Println(err)
	fmt.Printf("%+v\n", q.Base)
	fmt.Println(q.User, q.Level, q.When.Sec, q.When.Zone, len(q.Items), q.Items[1].Note != nil && *q.Items[1].Note == "fragile")
	fmt.Println(q.Tags, q.Meta["a"], q.ByLevel[3], q.Count, string(q.Raw), q.Num, q.Ptr == nil, q.Dur)
	ex, ok := q.Extra.(map[string]any)
	fmt.Println(ok)
	if ok {
		arr, ok := ex["k"].([]any)
		fmt.Println(ok, len(arr))
		if ok {
			f, isF := arr[0].(float64)
			s, isS := arr[1].(string)
			fmt.Println(f, isF, s, isS, arr[3] == nil, arr[4])
		}
	}
	n, err := q.Num.Float64()
	fmt.Println(n, err)

	var out bytes.Buffer
	if err := json.Indent(&out, b[:60], "", "  "); err != nil {
		fmt.Println("indent error:", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	enc.Encode(map[Level][]Stamp{0: {{1, "a"}}, 2: nil})
	enc.Encode([]any{Level(1), Celsius(2.5), Tags{"x"}, &p.Items[0]})

	dec := json.NewDecoder(strings.NewReader(`{"name":"a","price":1} {"name":"b","price":2,"unknown":1}`))
	dec.DisallowUnknownFields()
	for {
		var it Item
		if err := dec.Decode(&it); err != nil {
			fmt.Println("decode:", err)
			break
		}
		fmt.Printf("%+v\n", it)
	}

	var bad Payload
	err = json.Unmarshal([]byte(`{"level":"verbose"}`), &bad)
	fmt.Println("bad level:", err)
	err = json.Unmarshal([]byte(`{"user":`), &bad)
	var se *json.SyntaxError
	fmt.Println("syntax:", err, errors.As(err, &se), se != nil && se.Offset > 0)
	err = json.Unmarshal([]byte(`{"qty":"x"}`), &Item{})
	var ue *json.UnmarshalTypeError
	fmt.Println("type:", err, errors.As(err, &ue))
	err = json.Unmarshal([]byte(`{"when":"nosep"}`), &bad)
	fmt.Println("custom:", err)

	var anyv any
	json.Unmarshal([]byte(`[1,{"a":[true,null]},"s"]`), &anyv)
	fmt.Printf("%v %T\n", anyv, anyv)

	type local struct {
		A int `json:"a"`
		B []Level
	}
	lb, _ := json.MarshalIndent(local{A: 1, B: []Level{0, 3}}, "", "\t")
	fmt.Println(string(lb))
}

var benchPayload = []byte(`{"id":7,"user":"alice","level":"warn","when":"1700000000@UTC","items":[{"name":"kbd","price":49.5,"qty":1},{"name":"glass","price":3,"note":"fragile"}],"tags":["a","b"],"meta":{"a":"2","z":"1"},"by_level":{"info":10},"extra":{"k":[1,"two"]},"count":"42","num":12.5,"dur":1000000000}`)

// benchOp: decode then re-encode a payload with interpreted types.
func benchOp() {
	var p Payload
	if err := json.Unmarshal(benchPayload, &p); err != nil {
		panic(err)
	}
	if _, err := json.Marshal(p); err != nil {
		panic(err)
	}
}

// benchMain runs op AOT_BENCH times and prints the cost per call.
func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
