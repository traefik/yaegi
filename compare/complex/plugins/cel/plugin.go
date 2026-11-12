package celplug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/ext"
)

// state evaluates CEL policies (Google's Common Expression Language, used
// by Kubernetes and Envoy). cel-go is built on protobuf and ANTLR.
type state struct {
	progs []named
}

type named struct {
	name string
	prg  cel.Program
}

var policies = []struct{ name, src string }{
	{"api", `request.method == "POST" && request.path.startsWith("/api/")`},
	{"admin", `request.path.matches("^/admin") ? "deny" : "allow"`},
	{"headers", `size(request.headers) > 2 && "User-Agent" in request.headers`},
	{"ua", `request.headers.exists(k, k == "Authorization") ? request.headers["Authorization"].split(" ")[0] : "anon"`},
	{"query", `request.query.map(k, k + "=" + request.query[k]).sort().join(",")`},
	{"b64", `base64.encode(bytes(request.ip))`},
	{"opt", `request.?query.?foo.orValue("nofoo")`},
	{"math", `double(math.greatest([size(request.path), 3, 7])) + double(size(request.host)) / 2.0`},
	{"pathlen", `slow(request.path)`},
}

func setup(_ context.Context) (*state, error) {
	env, err := cel.NewEnv(
		cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
		ext.Strings(), ext.Math(), ext.Lists(), ext.Sets(),
		ext.Encoders(), cel.OptionalTypes(),
		cel.Function("slow",
			cel.Overload("slow_string", []*cel.Type{cel.StringType}, cel.IntType,
				cel.UnaryBinding(func(v ref.Val) ref.Val { return types.Int(len(v.(types.String)) * 10) }))),
	)
	if err != nil {
		return nil, err
	}
	s := &state{}
	for _, p := range policies {
		ast, iss := env.Compile(p.src)
		if iss.Err() != nil {
			return nil, fmt.Errorf("%s: %w", p.name, iss.Err())
		}
		prg, err := env.Program(ast, cel.EvalOptions(cel.OptOptimize))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.name, err)
		}
		s.progs = append(s.progs, named{p.name, prg})
	}
	if _, iss := env.Compile(`request.method + 1 == "x"`); iss.Err() == nil {
		return nil, fmt.Errorf("type error not detected")
	}
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	headers := map[string]string{}
	for k := range req.Header {
		headers[k] = req.Header.Get(k)
	}
	query := map[string]string{}
	for k := range req.URL.Query() {
		query[k] = req.URL.Query().Get(k)
	}
	act := map[string]any{"request": map[string]any{
		"method": req.Method, "path": req.URL.Path, "host": req.Host, "ip": ip,
		"headers": headers, "query": query,
	}}
	var out []string
	for _, p := range s.progs {
		v, _, err := p.prg.Eval(act)
		if err != nil {
			out = append(out, p.name+"=error("+err.Error()+")")
			continue
		}
		out = append(out, fmt.Sprintf("%s=%v", p.name, v.Value()))
	}
	return strings.Join(out, " "), nil
}
