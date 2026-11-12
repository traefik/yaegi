// json: UnmarshalJSON on nested fields, pointer fields, slice elements and
// map values.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Dur struct{ D time.Duration }

func (d *Dur) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	d.D = v
	return err
}

type Flag bool

func (f *Flag) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	*f = Flag(s == "yes" || s == "true")
	return nil
}

type Inner struct {
	Timeout Dur  `json:"timeout"`
	On      Flag `json:"on"`
}

type Config struct {
	In      Inner            `json:"in"`
	InPtr   *Inner           `json:"in_ptr"`
	Waits   []Dur            `json:"waits"`
	ByName  map[string]Dur   `json:"by_name"`
	PtrDur  *Dur             `json:"ptr_dur"`
	Flags   map[string]*Flag `json:"flags"`
	Nested  [][]Flag         `json:"nested"`
	Ignored string           `json:"-"`
}

func main() {
	in := `{
	"in": {"timeout": "1.5s", "on": "yes"},
	"in_ptr": {"timeout": "2m", "on": "no"},
	"waits": ["1ms", "3h"],
	"by_name": {"a": "10s"},
	"ptr_dur": "7s",
	"flags": {"x": "yes", "y": "no"},
	"nested": [["yes"], ["no", "true"]],
	"Ignored": "zzz"
}`
	var c Config
	err := json.Unmarshal([]byte(in), &c)
	fmt.Println(err)
	fmt.Println(c.In.Timeout.D, c.In.On)
	fmt.Println(c.InPtr.Timeout.D, c.InPtr.On)
	fmt.Println(c.Waits[0].D, c.Waits[1].D)
	fmt.Println(c.ByName["a"].D)
	fmt.Println(c.PtrDur.D)
	fmt.Println(*c.Flags["x"], *c.Flags["y"])
	fmt.Println(c.Nested)
	fmt.Printf("%q\n", c.Ignored)

	// Error from the method on an inner field.
	err = json.Unmarshal([]byte(`{"in":{"timeout":"bad"}}`), &c)
	fmt.Println(err)

	// Decoder with DisallowUnknownFields.
	dec := json.NewDecoder(strings.NewReader(`{"in":{"timeout":"1s"},"extra":1}`))
	dec.DisallowUnknownFields()
	var c2 Config
	fmt.Println(dec.Decode(&c2), c2.In.Timeout.D)
}
