// Package hostmemplug writes to objects of the host that standard library
// variables reach, without naming the variable in the write, so that the
// load-time check does not see it. Each write runs on its own and reports
// "refused" when the VM refuses it (a recovered panic) or "done" when it
// runs. TestHostMem checks the result and the state of the host.
package hostmemplug

import (
	"context"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go/doc"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// try runs f and says whether the VM refused it.
func try(w io.Writer, name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && strings.Contains(err.Error(), "writing to memory of the host") {
				fmt.Fprintf(w, "%s: refused\n", name)
				return
			}
			fmt.Fprintf(w, "%s: panic %v\n", name, r)
		}
	}()
	f()
	fmt.Fprintf(w, "%s: done\n", name)
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		evil := time.FixedZone("EVIL", 3600)
		// Closed at run time.
		try(w, "location", func() { *time.Now().Location() = *evil })
		try(w, "local copy", func() {
			l := time.Local
			*l = *evil
		})
		try(w, "utc copy", func() {
			u := time.UTC
			*u = *evil
		})
		try(w, "field", func() {
			t := unicode.Upper
			t.LatinOffset = 99
		})
		try(w, "map delete", func() {
			m := unicode.Categories
			delete(m, "Lu")
		})
		try(w, "map update", func() {
			m := unicode.Categories
			m["Lu"] = nil
		})
		try(w, "map clear", func() {
			m := unicode.Scripts
			clear(m)
		})
		try(w, "field address", func() {
			e := http.ErrNotSupported
			p := &e.ErrorString
			*p = "pwned"
		})
		try(w, "slice element", func() {
			s := doc.IllegalPrefixes
			s[0] = "pwned"
		})
		try(w, "slice append", func() {
			s := doc.IllegalPrefixes[:0]
			s = append(s, "pwned")
		})
		try(w, "slice copy", func() {
			s := doc.IllegalPrefixes
			copy(s, []string{"pwned"})
		})
		try(w, "reflect encoding", func() {
			reflect.ValueOf(base64.StdEncoding).Elem().Set(reflect.ValueOf(base64.URLEncoding).Elem())
		})
		try(w, "reflect eof", func() {
			reflect.ValueOf(io.EOF).Elem().Set(reflect.ValueOf(errors.New("pwned")).Elem())
		})
		try(w, "reflect field", func() {
			reflect.ValueOf(unicode.Upper).Elem().FieldByName("LatinOffset").SetInt(99)
		})
		try(w, "reflect map", func() {
			reflect.ValueOf(unicode.Scripts).SetMapIndex(reflect.ValueOf("Latin"), reflect.Value{})
		})
		// The elements of a host []byte: the VM keeps it as a Go []byte
		// (bytes.go), so each path that writes a []byte checks it.
		try(w, "bytes element", func() {
			b := net.IPv4bcast
			b[0] = 7
		})
		try(w, "bytes pointer", func() {
			b := net.IPv6loopback
			p := &b[15]
			*p = 7
		})
		try(w, "bytes append", func() {
			b := net.IPv4allsys[:0]
			b = append(b, 7)
		})
		try(w, "bytes append slice", func() {
			b := net.IPv4allrouter[:0]
			b = append(b, []byte{7, 7}...)
		})
		try(w, "bytes copy", func() {
			b := net.IPv4zero
			copy(b, []byte{7})
		})
		try(w, "bytes copy string", func() {
			b := net.IPv6zero
			copy(b, "x")
		})
		try(w, "bytes clear", func() {
			b := net.IPv6loopback
			clear(b)
		})
		try(w, "bytes reflect copy", func() {
			reflect.Copy(reflect.ValueOf(net.IPv6linklocalallrouters), reflect.ValueOf([]byte{7}))
		})
		try(w, "bytes reflect clear", func() {
			reflect.ValueOf(net.IPv6interfacelocalallnodes).Clear()
		})
		// Three bytes do not fit in the free capacity (2): as in Go, the
		// append makes a new array and writes nothing into the host one.
		try(w, "bytes reflect append overflow", func() {
			b := net.IPv4allsys[:14]
			r := reflect.Append(reflect.ValueOf(b), reflect.ValueOf(byte(7)), reflect.ValueOf(byte(7)), reflect.ValueOf(byte(7)))
			if r.Len() != 17 || string(r.Bytes()[14:]) != "\x07\x07\x07" {
				panic(fmt.Sprintf("append result: %v", r.Interface()))
			}
		})
		try(w, "bytes asn1", func() {
			b := asn1.NullBytes
			b[0] = 7
		})
		try(w, "bytes reflect index", func() {
			reflect.ValueOf(net.IPv4zero).Index(0).SetUint(7)
		})
		try(w, "bytes reflect elem", func() {
			b := net.IPv6zero
			reflect.ValueOf(&b[3]).Elem().SetUint(7)
		})
		try(w, "bytes reflect swapper", func() {
			reflect.Swapper(net.IPv6loopback)(0, 15)
		})
		try(w, "bytes sort", func() {
			b := net.IPv4allrouter
			sort.Slice(b, func(i, j int) bool { return b[i] > b[j] })
		})
		try(w, "bytes own", func() {
			b := make([]byte, 4)
			b[0] = 7
			p := &b[1]
			*p = 7
			b = append(b[:2], 7)
			copy(b, "xy")
			clear(b[2:])
			b = b[:4]
			reflect.Copy(reflect.ValueOf(b[3:]), reflect.ValueOf([]byte{7}))
			ip := net.IPv4(1, 2, 3, 4)
			ip[15] = 5
			reflect.ValueOf(ip).Index(14).SetUint(3)
			reflect.Swapper(ip)(14, 15)
			if string(b) != "xy\x00\x07" || ip.String() != "1.2.5.3" {
				panic(fmt.Sprintf("own bytes: %q %v", b, ip))
			}
		})
		// With Options.Stdout and Stderr nil, the plugin's os.Stdout and
		// os.Stderr are its own files (duplicates of the host's
		// descriptors, hostfd.go), so writing them changes nothing of the
		// host.
		try(w, "stdout object", func() {
			f := os.Stdout
			*f = os.File{}
		})
		try(w, "stderr object", func() {
			f := os.Stderr
			*f = *os.Stdin
		})
		// The plugin's own objects stay writable, and the identity of the
		// host errors stays.
		try(w, "own", func() {
			o := os.Stdout
			os.Stdout = os.Stderr
			os.Stdout = o
			c := &http.Client{}
			c.Timeout = time.Second
			loc := time.FixedZone("x", 1)
			*loc = *time.UTC
			h := http.Header{}
			h.Set("a", "b")
			reflect.ValueOf(c).Elem().FieldByName("Timeout").SetInt(2)
			m := map[string]*unicode.RangeTable{}
			for k, v := range unicode.Categories {
				m[k] = v
			}
			delete(m, "Lu")
			var e error = io.EOF
			_, err := strings.NewReader("").Read(make([]byte, 1))
			if err != e || !errors.Is(err, io.EOF) || io.EOF.Error() != "EOF" {
				panic("io.EOF changed")
			}
		})
		// The VM checks only its own writes (hostmem.go): this case pins
		// what a native decoder does with a pointer of the host.
		try(w, "hole json", func() {
			json.Unmarshal([]byte(`{"ErrorString":"pwned"}`), http.ErrNotSupported)
		})
	}), nil
}
