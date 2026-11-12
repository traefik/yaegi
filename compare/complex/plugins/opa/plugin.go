package opaplug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"
)

// state evaluates a Rego policy with Open Policy Agent, one of the largest
// Go dependencies a plugin can take (parser, compiler, topdown evaluator,
// builtins).
type state struct {
	q rego.PreparedEvalQuery
}

const policy = `
package traefik.authz

import rego.v1

default allow := false

allow if {
	input.method == "GET"
	not startswith(input.path, "/admin")
}

allow if {
	input.method == "POST"
	input.headers["X-Api-Key"] == data.keys[_]
}

reasons contains msg if {
	startswith(input.path, "/admin")
	msg := sprintf("admin path %s", [input.path])
}

reasons contains "private ip" if net.cidr_contains("10.0.0.0/8", input.ip)

reasons contains "jndi" if regex.match("\\$\\{jndi:", input.query)

segments := [s | some s in split(trim(input.path, "/"), "/"); s != ""]

header_count := count(input.headers)
`

func setup(ctx context.Context) (*state, error) {
	r := rego.New(
		rego.Query("data.traefik.authz"),
		rego.Module("authz.rego", policy),
		rego.Data(map[string]any{"keys": []any{"secret-key", "other"}}),
	)
	q, err := r.PrepareForEval(ctx)
	if err != nil {
		return nil, err
	}
	return &state{q: q}, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	headers := map[string]any{}
	for k := range req.Header {
		headers[k] = req.Header.Get(k)
	}
	input := map[string]any{"method": req.Method, "path": req.URL.Path, "ip": ip, "headers": headers, "query": req.URL.RawQuery}
	rs, err := s.q.Eval(req.Context(), rego.EvalInput(input))
	if err != nil {
		return "", err
	}
	if len(rs) != 1 {
		return "", fmt.Errorf("%d results", len(rs))
	}
	out := rs[0].Expressions[0].Value.(map[string]any)
	var reasons []string
	for _, r := range out["reasons"].([]any) {
		reasons = append(reasons, r.(string))
	}
	sort.Strings(reasons)
	return fmt.Sprintf("allow=%v reasons=%s segments=%v headers=%v", out["allow"], strings.Join(reasons, ";"),
		out["segments"], out["header_count"]), nil
}
