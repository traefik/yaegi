// Package providerplugin is a Traefik provider plugin, in the shape of the
// provider of traefik/plugindemo: New returns a provider whose Provide sends
// configurations from a goroutine until Stop.
//
// Config.Mode makes the plugin misbehave, for the tests of the sandbox:
// "newgoexit", "newloop" and "newpanic" act in New, "initgoexit", "initloop"
// and "initpanic" in Init, "goloop" and "gopanic" in the goroutine that
// Provide starts.
package providerplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"
)

type Config struct {
	PollInterval string
	Names        []string
	Mode         string
}

func CreateConfig() *Config {
	return &Config{PollInterval: "10ms"}
}

type Provider struct {
	name         string
	pollInterval time.Duration
	names        []string
	mode         string
	cancel       func()
}

// misbehave does what mode asks when it is the mode of the plugin.
func misbehave(mode, prefix string) {
	switch mode {
	case prefix + "goexit":
		runtime.Goexit()
	case prefix + "loop":
		n := 0
		for {
			if n++; n < 0 {
				break
			}
		}
	case prefix + "panic":
		panic("boom in " + prefix)
	}
}

func New(ctx context.Context, config *Config, name string) (*Provider, error) {
	misbehave(config.Mode, "new")
	pi, err := time.ParseDuration(config.PollInterval)
	if err != nil {
		return nil, err
	}
	if len(config.Names) == 0 {
		return nil, errors.New("no names")
	}
	return &Provider{name: name, pollInterval: pi, names: config.Names, mode: config.Mode}, nil
}

func (p *Provider) Init() error {
	misbehave(p.mode, "init")
	if p.pollInterval <= 0 {
		return fmt.Errorf("poll interval must be greater than 0")
	}
	return nil
}

func (p *Provider) Provide(cfgChan chan<- json.Marshaler) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go func() {
		misbehave(p.mode, "go")
		ticker := time.NewTicker(p.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cfgChan <- &JSONPayload{Configuration: p.generate()}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (p *Provider) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func (p *Provider) generate() *Configuration {
	cfg := &Configuration{HTTP: &HTTPConfiguration{Routers: map[string]*Router{}}}
	for _, n := range p.names {
		cfg.HTTP.Routers[n] = &Router{Service: "svc-" + n, Rule: "Host(`" + n + ".localhost`)"}
	}
	return cfg
}

// JSONPayload is the payload of genconf: its MarshalJSON is called by Traefik.
type JSONPayload struct {
	*Configuration
}

func (c JSONPayload) MarshalJSON() ([]byte, error) {
	if c.Configuration == nil {
		return nil, nil
	}
	return json.Marshal(c.Configuration)
}

type Configuration struct {
	HTTP *HTTPConfiguration `json:"http,omitempty"`
}

type HTTPConfiguration struct {
	Routers map[string]*Router `json:"routers,omitempty"`
}

type Router struct {
	Service string `json:"service,omitempty"`
	Rule    string `json:"rule,omitempty"`
}
