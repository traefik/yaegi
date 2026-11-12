// gopkg.in/yaml.v3: struct tags, inline, omitempty, flow, custom
// MarshalYAML and UnmarshalYAML (yaml.Node), TextUnmarshaler,
// interfaces holding interpreted types, errors.
package main

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	d.Duration = v
	return err
}

func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

type Level int

func (l *Level) UnmarshalText(b []byte) error {
	*l = Level(len(b))
	return nil
}

func (l Level) MarshalText() ([]byte, error) { return []byte(strings.Repeat("+", int(l))), nil }

type Common struct {
	Name string `yaml:"name"`
}

type Server struct {
	Common  `yaml:",inline"`
	Port    int               `yaml:"port"`
	Timeout Duration          `yaml:"timeout"`
	Lvl     Level             `yaml:"lvl"`
	Tags    []string          `yaml:"tags,flow"`
	Labels  map[string]string `yaml:"labels,omitempty"`
	Backend *Backend          `yaml:"backend,omitempty"`
	Extra   map[string]any    `yaml:",inline"`
	Any     any               `yaml:"any"`
}

type Backend struct {
	URLs    []string   `yaml:"urls"`
	Retries []Duration `yaml:"retries"`
}

func main() {
	in := `
name: web
port: 8080
timeout: 1m30s
lvl: high
tags: [a, b]
backend:
  urls: ["http://x", "http://y"]
  retries: [1s, 2s]
custom: 42
other: {k: v}
any: [1, two]
`
	var s Server
	err := yaml.Unmarshal([]byte(in), &s)
	fmt.Println(err)
	fmt.Println(s.Name, s.Port, s.Timeout.Duration, s.Lvl, s.Tags, s.Backend.URLs, s.Backend.Retries[1].Duration)
	fmt.Println(s.Extra["custom"], s.Extra["other"], s.Any)

	out, err := yaml.Marshal(s)
	fmt.Print(string(out))
	fmt.Println(err)

	var n yaml.Node
	_ = yaml.Unmarshal([]byte(in), &n)
	fmt.Println(n.Kind, len(n.Content[0].Content), n.Content[0].Content[0].Value)

	err = yaml.Unmarshal([]byte("port: notanint\ntimeout: bad\n"), &s)
	fmt.Println(err)
	var strict Backend
	dec := yaml.NewDecoder(strings.NewReader("urls: [a]\nunknown: 1\n"))
	dec.KnownFields(true)
	fmt.Println(dec.Decode(&strict))
}
