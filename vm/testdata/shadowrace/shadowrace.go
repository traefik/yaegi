// Package shadowrace is a plugin whose handler decodes JSON into a
// json.RawMessage field and encodes a shared config that holds one. In AOT
// mode the AOT copy of encoding/json reads and writes these fields through
// shadows of the VM cells (vm/twin_value.go, shadowRef): concurrent
// requests make and give back shadows at the same time, also on the same
// cells (the shared config).
package shadowrace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Config struct {
	Name string          `json:"name"`
	Keys json.RawMessage `json:"keys"`
}

func CreateConfig() *Config {
	return &Config{Name: "jwks", Keys: json.RawMessage(`[4,5]`)}
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[1:]
		var doc struct {
			Alg  string          `json:"alg"`
			Keys json.RawMessage `json:"keys"`
		}
		if err := json.Unmarshal([]byte(`{"alg":"RS`+id+`","keys":[`+id+`,2,3]}`), &doc); err != nil {
			fmt.Fprint(w, "error: ", err)
			return
		}
		b, err := json.Marshal(cfg)
		if err != nil {
			fmt.Fprint(w, "error: ", err)
			return
		}
		fmt.Fprintf(w, "%s %s %s", doc.Alg, doc.Keys, b)
	}), nil
}
