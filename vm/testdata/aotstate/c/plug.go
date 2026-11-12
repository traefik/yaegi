// Package plug (version c) goes through the AOT copies of encoding/xml and
// encoding/binary, whose caches are indexed by type.
package plug

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"net/http"
	"strconv"
)

type Config struct{ Name string }

type Payload struct {
	Name  string
	Count int
}

type header struct {
	Magic uint16
	Len   uint32
}

func CreateConfig() *Config { return &Config{Name: "c"} }

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := xml.Marshal(Payload{Name: cfg.Name, Count: 3})
		var back Payload
		xml.Unmarshal(b, &back)
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, header{Magic: 0xCAFE, Len: uint32(len(b))})
		w.Write(b)
		w.Write([]byte(" " + back.Name + " " + strconv.Itoa(binary.Size(header{}))))
	}), nil
}
