// AOT test program: github.com/goccy/go-yaml on interpreted types.
//
//aot:lib github.com/goccy/go-yaml
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

type Mode int

func (m Mode) MarshalYAML() ([]byte, error) { return []byte([...]string{"off", "on"}[m]), nil }
func (m *Mode) UnmarshalYAML(b []byte) error {
	switch strings.TrimSpace(string(b)) {
	case "off":
		*m = 0
	case "on":
		*m = 1
	default:
		return fmt.Errorf("bad mode %q", b)
	}
	return nil
}

type Endpoint struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func (e Endpoint) String() string { return e.Host + ":" + strconv.Itoa(e.Port) }

type Base struct {
	ID int `yaml:"id"`
}

type Config struct {
	Base      `yaml:",inline"`
	Mode      Mode              `yaml:"mode"`
	Endpoints []Endpoint        `yaml:"endpoints"`
	Labels    map[string]string `yaml:"labels"`
	Any       any               `yaml:"any"`
	Opt       *Endpoint         `yaml:"opt,omitempty"`
	Wait      time.Duration     `yaml:"wait"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	c := Config{Base: Base{7}, Mode: 1, Endpoints: []Endpoint{{"a", 1}, {"b", 2}},
		Labels: map[string]string{"z": "1", "a": "2"}, Any: []any{1, "x"}, Wait: 2 * time.Second}
	b, err := yaml.Marshal(c)
	fmt.Print(string(b))
	fmt.Println(err)
	src := "id: 3\nmode: off\nendpoints:\n  - host: h\n    port: 80\nlabels: {k: v}\nany: {n: [1, 2]}\nopt: {host: o, port: 9}\nwait: 1s\n"
	var d Config
	err = yaml.Unmarshal([]byte(src), &d)
	fmt.Println(err, d.ID, d.Mode, d.Endpoints, d.Labels, d.Opt, d.Wait)
	fmt.Printf("%v %T\n", d.Any, d.Any)
	err = yaml.Unmarshal([]byte("mode: maybe"), &d)
	fmt.Println(err != nil)
	err = yaml.Unmarshal([]byte("endpoints: {x: 1}"), &d)
	fmt.Println(err != nil)
	j, err := yaml.YAMLToJSON([]byte("a: [1, {b: c}]"))
	fmt.Println(string(j), err)
}

const benchDoc = "id: 3\nmode: on\nendpoints:\n  - host: h\n    port: 80\n  - host: i\n    port: 81\nlabels: {k: v}\nwait: 1s\n"

// benchOp: unmarshal then marshal a config.
func benchOp() {
	var c Config
	if err := yaml.Unmarshal([]byte(benchDoc), &c); err != nil {
		panic(err)
	}
	if _, err := yaml.Marshal(c); err != nil {
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
