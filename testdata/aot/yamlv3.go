// AOT test program: gopkg.in/yaml.v3 on interpreted types.
//
//aot:lib gopkg.in/yaml.v3
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Level int

func (l Level) MarshalText() ([]byte, error) { return []byte([...]string{"low", "high"}[l]), nil }
func (l *Level) UnmarshalText(b []byte) error {
	switch string(b) {
	case "low":
		*l = 0
	case "high":
		*l = 1
	default:
		return fmt.Errorf("bad level %q", b)
	}
	return nil
}

// Port uses the yaml.v3 Marshaler/Unmarshaler interfaces.
type Port struct{ N int }

func (p Port) MarshalYAML() (any, error) { return fmt.Sprintf("port-%d", p.N), nil }
func (p *Port) UnmarshalYAML(n *yaml.Node) error {
	s := strings.TrimPrefix(n.Value, "port-")
	v, err := strconv.Atoi(s)
	p.N = v
	return err
}

// Legacy uses the yaml.v2-style unmarshaler.
type Legacy struct{ Parts []string }

func (l *Legacy) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	l.Parts = strings.Split(s, "/")
	return nil
}

type Common struct {
	Name string `yaml:"name"`
	Tags []string `yaml:"tags,flow"`
}

type Config struct {
	Common  `yaml:",inline"`
	Level   Level             `yaml:"level"`
	Ports   []Port            `yaml:"ports"`
	Limits  map[string]int    `yaml:"limits"`
	Extra   map[string]any    `yaml:"extra,omitempty"`
	Path    Legacy            `yaml:"path"`
	Timeout time.Duration     `yaml:"timeout"`
	Ptr     *Common           `yaml:"ptr"`
	Empty   string            `yaml:"empty,omitempty"`
	Rest    map[string]string `yaml:",inline"`
	hidden  int
}

func main() {
	if benchMain(benchOp) {
		return
	}
	c := Config{Common: Common{"svc", []string{"a", "b"}}, Level: 1, Ports: []Port{{80}, {443}},
		Limits: map[string]int{"cpu": 2, "mem": 512}, Extra: map[string]any{"x": []any{1, "y"}},
		Timeout: 3 * time.Second, Ptr: &Common{Name: "p"}, Rest: map[string]string{"zz": "top"}}
	b, err := yaml.Marshal(c)
	fmt.Print(string(b))
	fmt.Println(err)
	src := `name: api
tags: [x, y]
level: low
ports: [port-8080, port-9090]
limits: {cpu: 4}
extra:
  list: [1, two, 3.5, true, null]
  map: {k: v}
path: usr/local/bin
timeout: 1m30s
ptr: {name: inner}
other: kept
anchor: &a hello
alias: *a
`
	var d Config
	err = yaml.Unmarshal([]byte(src), &d)
	fmt.Println(err)
	fmt.Println(d.Name, d.Tags, d.Level, d.Ports, d.Limits, d.Path.Parts, d.Timeout, d.Ptr.Name, d.Rest["other"], d.Rest["alias"])
	fmt.Printf("%v %T\n", d.Extra["list"], d.Extra["map"])
	var node yaml.Node
	yaml.Unmarshal([]byte("a: 1\nb: [2, 3]\n"), &node)
	fmt.Println(node.Kind == yaml.DocumentNode, len(node.Content[0].Content))
	var m map[string]Port
	err = yaml.Unmarshal([]byte("x: port-1\ny: port-2"), &m)
	fmt.Println(err, m)
	err = yaml.Unmarshal([]byte("level: medium"), &d)
	fmt.Println(err)
	err = yaml.Unmarshal([]byte("ports: [port-x]"), &d)
	fmt.Println(err)
	var te *yaml.TypeError
	err = yaml.Unmarshal([]byte("limits: [1]"), &d)
	fmt.Println(errors.As(err, &te), err)
	dec := yaml.NewDecoder(strings.NewReader("name: a\nunknown: 1\n"))
	dec.KnownFields(true)
	fmt.Println(dec.Decode(&Common{}))
	var anyv any
	yaml.Unmarshal([]byte("[1, {a: b}]"), &anyv)
	fmt.Printf("%v %T\n", anyv, anyv)
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	enc.Encode(map[Level][]Port{1: {{1}}})
	enc.Close()
}

const benchDoc = `name: api
tags: [x, y]
level: high
ports: [port-8080, port-9090]
limits: {cpu: 4, mem: 256}
path: usr/local/bin
timeout: 1m30s
`

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
