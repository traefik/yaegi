package exprplug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Env is what the rules see. expr reads it with reflect: fields, methods,
// maps and slices of a type of the plugin.
type Env struct {
	Method  string
	Path    string
	Host    string
	IP      string
	Headers map[string]string
	Query   map[string][]string
	Tags    []string
}

// HasPrefix is a method of the environment, called by the rules.
func (e Env) HasPrefix(p string) bool { return strings.HasPrefix(e.Path, p) }

// Score is a method with a struct result.
func (e Env) Score() Score { return Score{Value: len(e.Path) + len(e.Headers)} }

// Score is returned by a method of Env.
type Score struct{ Value int }

type rule struct {
	name string
	prog *vm.Program
}

type state struct{ rules []rule }

var sources = []struct{ name, src string }{
	{"api", `Method == "POST" and HasPrefix("/api/")`},
	{"browser", `Headers["User-Agent"] contains "Mozilla"`},
	{"private", `IP startsWith "10." or IP in ["127.0.0.1"]`},
	{"query", `len(Query) > 0 ? join(map(sort(keys(Query)), {# + "=" + Query[#][0]}), "&") : "none"`},
	{"score", `Score().Value * 2 + count(Tags, {len(#) > 3})`},
	{"jndi", `any(values(Query), {any(#, {# matches "\\$\\{jndi:"})})`},
	{"upper", `upper(Host) + ":" + string(len(Headers))`},
}

func setup(_ context.Context) (*state, error) {
	s := &state{}
	for _, r := range sources {
		p, err := expr.Compile(r.src, expr.Env(Env{}))
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.name, err)
		}
		s.rules = append(s.rules, rule{r.name, p})
	}
	// A rule with a type error must be refused at compile time.
	if _, err := expr.Compile(`Method + 1`, expr.Env(Env{}), expr.AsBool()); err == nil {
		return nil, fmt.Errorf("type error not detected")
	}
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	env := Env{Method: req.Method, Path: req.URL.Path, Host: req.Host, IP: ip, Headers: map[string]string{},
		Query: req.URL.Query(), Tags: []string{"edge", "eu", "plugin"}}
	for k := range req.Header {
		env.Headers[k] = req.Header.Get(k)
	}
	var out []string
	for _, r := range s.rules {
		v, err := expr.Run(r.prog, env)
		if err != nil {
			return "", fmt.Errorf("rule %s: %w", r.name, err)
		}
		out = append(out, fmt.Sprintf("%s=%v", r.name, v))
	}
	// A map environment, no struct.
	v, err := expr.Eval(`a + b * 2`, map[string]any{"a": 1, "b": len(req.Header)})
	if err != nil {
		return "", err
	}
	out = append(out, fmt.Sprintf("eval=%v", v))
	return strings.Join(out, " "), nil
}
