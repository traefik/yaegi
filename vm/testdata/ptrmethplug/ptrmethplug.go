// Package ptrmethplug decodes JSON into types with pointer methods defined
// in a subpackage: before a fix, they stayed
// empty and json returned no error.
package ptrmethplug

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/traefik/yaegi/v2/vm/testdata/ptrmethplug/wire"
)

type Config struct {
	Index string
}

func CreateConfig() *Config { return &Config{Index: `{"bot":1,"crawler":2}`} }

type plug struct {
	next  http.Handler
	index wire.Index
}

func New(_ context.Context, next http.Handler, cfg *Config, _ string) (http.Handler, error) {
	var idx wire.Index
	if err := json.Unmarshal([]byte(cfg.Index), &idx); err != nil {
		return nil, err
	}
	if err := idx.Validate(); err != nil {
		return nil, err
	}
	return &plug{next: next, index: idx}, nil
}

func (p *plug) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var out []string
	out = append(out, "index "+p.index.Keys())

	resp := &wire.Response{Body: []byte("old")}
	err := resp.FromJSON([]byte(`{"status":201,"headers":{"A":"1"},"body":"aGk="}`))
	out = append(out, fmt.Sprintf("response %v %d %v %s", err, resp.Status, resp.Headers, resp.Body))
	b, err := resp.ToJSON()
	out = append(out, fmt.Sprintf("tojson %v %s", err, b))

	var dec wire.Response
	err = json.NewDecoder(r.Body).Decode(&dec)
	out = append(out, fmt.Sprintf("decode %v %d %v", err, dec.Status, dec.Headers))

	var n wire.Named
	err = json.Unmarshal([]byte(`{"Name":"n"}`), &n)
	out = append(out, fmt.Sprintf("named %v %v", err, &n))

	var u wire.Upper
	err = json.Unmarshal([]byte(`"up"`), &u)
	out = append(out, fmt.Sprintf("upper %v %s", err, u.V))

	idx := wire.Index{"old": 0}
	alias := idx
	err = json.Unmarshal([]byte(`{"new":1}`), &idx)
	out = append(out, fmt.Sprintf("index2 %v %v %s", err, idx.Validate(), alias.Keys()))

	fmt.Fprint(w, strings.Join(out, "\n"))
}
