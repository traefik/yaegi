// AOT test program: github.com/pelletier/go-toml/v2 on interpreted types.
//
//aot:lib github.com/pelletier/go-toml/v2
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Color int

func (c Color) MarshalText() ([]byte, error) { return []byte([...]string{"red", "blue"}[c]), nil }
func (c *Color) UnmarshalText(b []byte) error {
	switch string(b) {
	case "red":
		*c = 0
	case "blue":
		*c = 1
	default:
		return fmt.Errorf("bad color %q", b)
	}
	return nil
}

type DB struct {
	URL     string        `toml:"url"`
	Pool    int           `toml:"pool,omitempty"`
	Timeout time.Duration `toml:"timeout"`
}

type App struct {
	Name   string            `toml:"name"`
	Color  Color             `toml:"color"`
	DB     DB                `toml:"db"`
	Tags   []string          `toml:"tags"`
	Env    map[string]string `toml:"env"`
	Points [][]int           `toml:"points"`
	Opt    *DB               `toml:"opt"`
	Extra  any               `toml:"extra"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	a := App{Name: "x", Color: 1, DB: DB{"pg://", 4, time.Second}, Tags: []string{"a"},
		Env: map[string]string{"B": "2", "A": "1"}, Points: [][]int{{1, 2}, {3}}}
	b, err := toml.Marshal(a)
	fmt.Print(string(b))
	fmt.Println(err)
	var c App
	err = toml.Unmarshal([]byte(`name = "y"
color = "red"
tags = ["p", "q"]
points = [[5]]
extra = {k = [1, "two"]}
[db]
url = "my://"
timeout = 1500000000
[opt]
url = "o"
[env]
K = "V"
`), &c)
	fmt.Println(err, c.Name, c.Color, c.DB, c.Tags, c.Env, c.Points, c.Opt.URL)
	fmt.Printf("%v %T\n", c.Extra, c.Extra)
	err = toml.Unmarshal([]byte(`color = "green"`), &c)
	fmt.Println(err)
	err = toml.Unmarshal([]byte(`name = 1`), &c)
	var de *toml.DecodeError
	fmt.Println(errors.As(err, &de))
	d := toml.NewDecoder(strings.NewReader("name = \"n\"\nbogus = 1\n"))
	d.DisallowUnknownFields()
	err = d.Decode(&c)
	var sm *toml.StrictMissingError
	fmt.Println(errors.As(err, &sm))
}

const benchDoc = `name = "y"
color = "blue"
tags = ["p", "q"]
[db]
url = "my://"
pool = 3
timeout = 1500000000
[env]
K = "V"
`

// benchOp: unmarshal then marshal an app config.
func benchOp() {
	var a App
	if err := toml.Unmarshal([]byte(benchDoc), &a); err != nil {
		panic(err)
	}
	if _, err := toml.Marshal(a); err != nil {
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
