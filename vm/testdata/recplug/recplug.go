// Package recplug has a recursive config, like the RawRule of
// smerschjohann/mtlswhitelist: a rule holds sub-rules
// (a slice), an optional fallback rule (a pointer) and named rules (a map).
// The handler prints the decoded rules. On POST, it decodes the request
// body as a rule with json.Unmarshal and prints it.
package recplug

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Rule struct {
	Type     string          `json:"type,omitempty"`
	Ranges   []string        `json:"ranges,omitempty"`
	Port     int             `json:"port,omitempty"`
	Rules    []Rule          `json:"rules,omitempty"`
	Fallback *Rule           `json:"fallback,omitempty"`
	Named    map[string]Rule `json:"named,omitempty"`
}

type Config struct {
	Rules []Rule `json:"rules,omitempty"`
	Top   *Rule  `json:"top,omitempty"`
}

func CreateConfig() *Config { return &Config{} }

type handler struct{ out string }

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	var b strings.Builder
	for _, r := range cfg.Rules {
		write(&b, r)
		b.WriteString(";")
	}
	if cfg.Top != nil {
		b.WriteString("top=")
		write(&b, *cfg.Top)
	}
	return &handler{out: b.String()}, nil
}

func write(b *strings.Builder, r Rule) {
	fmt.Fprintf(b, "%s%v:%d(", r.Type, r.Ranges, r.Port)
	for _, s := range r.Rules {
		write(b, s)
	}
	b.WriteString(")")
	if r.Fallback != nil {
		b.WriteString("else ")
		write(b, *r.Fallback)
	}
	for _, k := range []string{"a", "b"} {
		if s, ok := r.Named[k]; ok {
			b.WriteString(k + "=")
			write(b, s)
		}
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		body, _ := io.ReadAll(req.Body)
		var r Rule
		if err := json.Unmarshal(body, &r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var b strings.Builder
		write(&b, r)
		fmt.Fprint(w, b.String())
		return
	}
	fmt.Fprint(w, h.out)
}
