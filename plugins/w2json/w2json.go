// Package w2json is a typical "body rewriting" Traefik plugin: it decodes a
// JSON request body into its own types, modifies it, and re-encodes it.
package w2json

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Config is the plugin configuration.
type Config struct {
	PriceFactor float64 `json:"priceFactor,omitempty"`
	Tag         string  `json:"tag,omitempty"`
	Stamp       int64   `json:"stamp,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{PriceFactor: 1}
}

// Level is a named non-struct type with a String method.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	}
	return "level(" + strconv.Itoa(int(l)) + ")"
}

// Stamp has a custom JSON encoding and is nested in Payload.
type Stamp struct {
	Sec  int64
	Zone string
}

// MarshalJSON implements json.Marshaler.
func (s Stamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(s.Sec, 10) + "@" + s.Zone + `"`), nil
}

// Item is a line of the payload.
type Item struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Price float64  `json:"price"`
	Qty   int      `json:"qty"`
	Tags  []string `json:"tags,omitempty"`
}

// Payload is the request body.
type Payload struct {
	User      string            `json:"user"`
	Level     Level             `json:"level"`
	LevelName string            `json:"levelName,omitempty"`
	Active    bool              `json:"active"`
	Items     []Item            `json:"items"`
	Meta      map[string]string `json:"meta"`
	Total     float64           `json:"total"`
	When      Stamp             `json:"when"`
	Note      *string           `json:"note,omitempty"`
}

// Rewriter is the middleware.
type Rewriter struct {
	next   http.Handler
	name   string
	factor float64
	tag    string
	stamp  int64
}

// New creates the middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if config.PriceFactor <= 0 {
		return nil, fmt.Errorf("invalid price factor %v", config.PriceFactor)
	}
	return &Rewriter{next: next, name: name, factor: config.PriceFactor, tag: config.Tag, stamp: config.Stamp}, nil
}

func (r *Rewriter) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(rw, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	total := 0.0
	for i := range p.Items {
		it := &p.Items[i]
		it.Price = float64(int(it.Price*r.factor*100)) / 100
		total += it.Price * float64(it.Qty)
		if r.tag != "" {
			it.Tags = append(it.Tags, r.tag)
		}
	}
	p.Total = total
	p.LevelName = p.Level.String()
	if p.Meta == nil {
		p.Meta = map[string]string{}
	}
	p.Meta["processedBy"] = r.name
	p.When = Stamp{Sec: r.stamp, Zone: "UTC"}
	note := "rewritten"
	p.Note = &note

	out, err := json.Marshal(p)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("X-Summary", fmt.Sprintf("%s/%d items", p.Level, len(p.Items)))
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(out)
}
