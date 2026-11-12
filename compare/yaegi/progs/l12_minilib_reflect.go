package main

import (
	"errors"
	"fmt"
	"strings"

	"example.com/minicfg"
)

type Mode string

func (m *Mode) UnmarshalText(b []byte) error {
	*m = Mode(strings.ToUpper(string(b)))
	return nil
}

type DB struct {
	Host string `env:"HOST"`
	Port int    `env:"PORT"`
}

type Cfg struct {
	Name  string   `env:"NAME"`
	Debug bool     `env:"DEBUG"`
	Mode  Mode     `env:"MODE"`
	Hosts []string `env:"HOSTS"`
	DB    DB       `env:"DB_"`
	Retry int
}

func (c *Cfg) Defaults() {
	if c.Retry == 0 {
		c.Retry = 3
	}
}

func (c *Cfg) Validate() error {
	if c.DB.Port == 0 {
		return errors.New("db port required")
	}
	return nil
}

func main() {
	var c Cfg
	err := minicfg.Load(&c, map[string]string{"NAME": "svc", "DEBUG": "true", "MODE": "fast", "HOSTS": "a,b", "DB_HOST": "db", "DB_PORT": "5432"})
	fmt.Printf("%v %+v\n", err, c)
	var bad Cfg
	fmt.Println(minicfg.Load(&bad, map[string]string{"NAME": "x"}))
	fmt.Println(minicfg.Describe(c.DB))
}
