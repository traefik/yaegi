// AOT test program: github.com/BurntSushi/toml on interpreted types.
//
//aot:lib github.com/BurntSushi/toml
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Size int64

func (s Size) MarshalText() ([]byte, error) { return []byte(strconv.FormatInt(int64(s)/1024, 10) + "KiB"), nil }
func (s *Size) UnmarshalText(b []byte) error {
	n, err := strconv.ParseInt(strings.TrimSuffix(string(b), "KiB"), 10, 64)
	*s = Size(n * 1024)
	return err
}

type Server struct {
	Host string   `toml:"host"`
	Port int      `toml:"port"`
	Tags []string `toml:"tags"`
}

type Config struct {
	Title   string            `toml:"title"`
	Owner   struct{ Name string } `toml:"owner"`
	Servers map[string]Server `toml:"servers"`
	Limit   Size              `toml:"limit"`
	Ratio   float64           `toml:"ratio"`
	Enabled bool              `toml:"enabled"`
	When    time.Time         `toml:"when"`
	Nested  []Server          `toml:"nested"`
	Any     map[string]any    `toml:"any"`
	Skip    string            `toml:"-"`
}

const doc = `
title = "demo"
limit = "64KiB"
ratio = 0.5
enabled = true
when = 2024-05-06T07:08:09Z

[owner]
Name = "Tom"

[servers.alpha]
host = "10.0.0.1"
port = 8001
tags = ["a", "b"]

[servers.beta]
host = "10.0.0.2"
port = 8002

[[nested]]
host = "n1"

[[nested]]
host = "n2"
port = 2

[any]
x = 1
y = [1, 2]
z = { k = "v" }
`

func main() {
	if benchMain(benchOp) {
		return
	}
	var c Config
	md, err := toml.Decode(doc, &c)
	fmt.Println(err, c.Title, c.Owner.Name, c.Servers["alpha"], c.Servers["beta"].Port, c.Limit, c.Ratio, c.Enabled, c.When.Year(), len(c.Nested), c.Nested[1])
	fmt.Printf("%v %T %T\n", c.Any["y"], c.Any["x"], c.Any["z"])
	fmt.Println(md.IsDefined("servers", "alpha", "tags"), len(md.Undecoded()))
	var buf bytes.Buffer
	err = toml.NewEncoder(&buf).Encode(c)
	fmt.Print(buf.String())
	fmt.Println(err)
	_, err = toml.Decode("limit = \"x\"", &c)
	fmt.Println(err)
	_, err = toml.Decode("title = 1", &c)
	var pe toml.ParseError
	fmt.Println(err, errors.As(err, &pe))
	_, err = toml.Decode("= broken", &c)
	fmt.Println(errors.As(err, &pe), pe.Position.Line)
	var m map[string]any
	_, err = toml.Decode("a = 1\n[b]\nc = 'd'", &m)
	fmt.Println(err, m)
}

// benchOp: decode then encode a config.
func benchOp() {
	var c Config
	if _, err := toml.Decode(doc, &c); err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
