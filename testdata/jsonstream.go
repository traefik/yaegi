package main

// json.Encoder and json.Decoder with interpreted types in default mode:
// they go to the AOT copy of json (aotroutejson.go), so the methods of the
// interpreted types are called, as in Go. A Decoder that decoded native
// values first gives its buffered data to the copy.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Level int

func (l Level) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote("L" + strconv.Itoa(int(l)))), nil
}

func (l *Level) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "L"))
	*l = Level(n)
	return err
}

type Event struct {
	Name  string  `json:"name"`
	Level Level   `json:"level"`
	Tags  []Level `json:"tags,omitempty"`
	HTML  string  `json:"html,omitempty"`
}

type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.n++
	if w.n > 1 {
		return 0, errors.New("disk full")
	}
	return len(p), nil
}

func main() {
	// Encoder: several values, indent, escapeHTML
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	fmt.Println(enc.Encode(Event{Name: "a", Level: 1, Tags: []Level{2, 3}}))
	fmt.Println(enc.Encode(map[string]int{"plain": 1}))
	enc.SetIndent(">", "  ")
	enc.SetEscapeHTML(false)
	fmt.Println(enc.Encode([]any{Level(4), Event{Name: "b", HTML: "<b>&</b>"}}))
	fmt.Print(buf.String())

	// a write error stays in the Encoder, as in Go
	fw := &failWriter{}
	enc = json.NewEncoder(fw)
	fmt.Println(enc.Encode(Level(1)), enc.Encode(Level(2)), enc.Encode(Level(3)), enc.Encode(4))

	// Decoder: a stream of values, native ones first, then interpreted ones
	in := `{"plain": 1} {"name":"x","level":"L5","tags":["L6"]} [{"name":"y","level":"L7"},{"name":"z","level":"L8"}] {"name":"w","level":"L9"} 42 "tail"`
	dec := json.NewDecoder(strings.NewReader(in))
	var plain map[string]int
	fmt.Println(dec.Decode(&plain), plain, dec.InputOffset())
	var ev Event
	fmt.Println(dec.Decode(&ev), ev, dec.InputOffset())
	tok, err := dec.Token()
	_, isDelim := tok.(json.Delim)
	fmt.Println(tok, err, isDelim)
	for dec.More() {
		var e Event
		fmt.Println(dec.Decode(&e), e)
	}
	tok, err = dec.Token()
	fmt.Println(tok, err, tok == json.Delim(']'))
	var evs []Event
	var last Event
	fmt.Println(dec.Decode(&last), last, evs == nil)
	var n int
	fmt.Println(dec.Decode(&n), n)
	rest, _ := io.ReadAll(dec.Buffered())
	fmt.Printf("%q %d\n", rest, dec.InputOffset())
	var s string
	fmt.Println(dec.Decode(&s), s, dec.Decode(&s) == io.EOF)

	// options and errors
	dec = json.NewDecoder(strings.NewReader(`{"name":"a","extra":1} {bad`))
	dec.DisallowUnknownFields()
	fmt.Println(dec.Decode(&ev))
	var se *json.SyntaxError
	err = dec.Decode(&ev)
	fmt.Println(errors.As(err, &se), err)

	dec = json.NewDecoder(strings.NewReader(`{"level":"L1","n":1.5} {"level":"L2","n":2}`))
	var m1 struct {
		Level Level
		N     any
	}
	fmt.Println(dec.Decode(&m1), m1.Level, m1.N)
	dec.UseNumber()
	fmt.Println(dec.Decode(&m1), m1.Level)
	_, isNum := m1.N.(json.Number)
	fmt.Println(m1.N, isNum)
}
