// Package wire has the types of ptrmethplug, in a subpackage: types with
// pointer methods that the plugin decodes with encoding/json.
package wire

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Response is shaped like the Response of negasus/traefik-plugin-bridge.
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

// Index is shaped like the user agent index of
// holysoles/bot-wrangler-traefik-plugin: a map with a pointer method.
type Index map[string]int

func (m *Index) Validate() error {
	if len(*m) == 0 {
		return errors.New("empty index")
	}
	return nil
}

func (m Index) Keys() string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// Named has a String method: it crosses to native code as a hand-written
// wrapper, except as the destination of a decoder.
type Named struct{ Name string }

func (n *Named) String() string { return "Named(" + n.Name + ")" }

// Upper has an UnmarshalJSON method, which json must call.
type Upper struct{ V string }

func (u *Upper) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	u.V = strings.ToUpper(s)
	return nil
}
