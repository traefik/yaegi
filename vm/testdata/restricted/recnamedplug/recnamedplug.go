// Package recnamedplug uses recursive named types that are not structs
// (type Tree map[string]Tree, type List []List), shaped like JSON trees. In
// default mode, their native type never ended and the first value that
// crossed to native code (DecodeConfig, json, fmt) killed the host with a
// fatal Go stack overflow.
package recnamedplug

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Tree map[string]Tree

type List []List

type Config struct {
	Tree  Tree
	Lists List
}

func CreateConfig() *Config { return &Config{} }

type plug struct {
	cfg *Config
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return &plug{cfg: cfg}, nil
}

func (p *plug) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/config":
		fmt.Fprint(w, p.cfg.Tree, " ", p.cfg.Lists)
	case "/cycle":
		t := Tree{}
		t["self"] = t
		_, err := json.Marshal(t)
		fmt.Fprint(w, err != nil)
	default:
		body, _ := io.ReadAll(r.Body)
		var t Tree
		err := json.Unmarshal(body, &t)
		b, _ := json.Marshal(t)
		fmt.Fprintf(w, "%v %s %v", t, b, err != nil)
	}
}
