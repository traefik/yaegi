package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ctxCheck reads the values of plugin A that are nested in maps or of
// native named types. The host runs it natively; plugin B runs the same
// text (ctxCheckSrc): both must give what Go gives.
func ctxCheck(ctx context.Context, set func(string, ...any)) {
	jwt, ok := ctx.Value("jwt").(map[string]any)
	g, gok := jwt["groups"].([]any)
	o, ook := jwt["o"].(map[string]any)
	set("jwt", ok, gok, g, ook, o, jwt["sub"])
	mx, ok := ctx.Value("mx").(map[string]any)
	roles, rok := mx["roles"].([]string)
	mm, mok := mx["m"].(map[string]string)
	n, nok := mx["n"].([]map[string]any)
	p, pok := mx["p"].(interface{ String() string })
	ps := ""
	if pok {
		ps = p.String()
	}
	set("mx", ok, rok, roles, mok, mm, nok, n, pok, ps)
	tm, ok := ctx.Value("time").(time.Time)
	set("time", ok, tm.Year(), tm.Second())
	d, ok := ctx.Value("d").(time.Duration)
	set("d", ok, int64(d))
	u, ok := ctx.Value("url").(*url.URL)
	if ok {
		set("url", ok, u.Path, u.Query().Get("q"))
	} else {
		set("url", ok)
	}
	ip, ok := ctx.Value("ip").(net.IP)
	set("ip", ok, ip.String())
	ck, ok := ctx.Value("cookie").(*http.Cookie)
	if ok {
		set("cookie", ok, ck.Value)
	} else {
		set("cookie", ok)
	}
	num, ok := ctx.Value("num").(json.Number)
	set("num", ok, string(num))
	hd, ok := ctx.Value("hdr").(http.Header)
	set("hdr", ok, hd.Get("K"))
	vals, ok := ctx.Value("vals").(url.Values)
	set("vals", ok, vals.Get("q"))
	req, ok := ctx.Value("req").(*http.Request)
	if ok {
		set("req", ok, req.Method)
	} else {
		set("req", ok)
	}
	sm, ok := ctx.Value("smap").(*sync.Map)
	if ok {
		v, _ := sm.Load("k")
		set("smap", ok, v)
	} else {
		set("smap", ok)
	}
	av, ok := ctx.Value("aval").(*atomic.Value)
	if ok {
		set("aval", ok, av.Load())
	} else {
		set("aval", ok)
	}
	buf, ok := ctx.Value("buf").(*bytes.Buffer)
	if ok {
		set("buf", ok, buf.String())
	} else {
		set("buf", ok)
	}
	sb, ok := ctx.Value("sb").(*strings.Builder)
	if ok {
		set("sb", ok, sb.String())
	} else {
		set("sb", ok)
	}
	raw, ok := ctx.Value("raw").(json.RawMessage)
	set("raw", ok, string(raw))
}

// ctxCheckSrc is ctxCheck as plugin source.
const ctxCheckSrc = `// ctxCheck reads the values of plugin A that are nested in maps or of
// native named types. The host runs it natively; plugin B runs the same
// text (ctxCheckSrc): both must give what Go gives.
func ctxCheck(ctx context.Context, set func(string, ...any)) {
	jwt, ok := ctx.Value("jwt").(map[string]any)
	g, gok := jwt["groups"].([]any)
	o, ook := jwt["o"].(map[string]any)
	set("jwt", ok, gok, g, ook, o, jwt["sub"])
	mx, ok := ctx.Value("mx").(map[string]any)
	roles, rok := mx["roles"].([]string)
	mm, mok := mx["m"].(map[string]string)
	n, nok := mx["n"].([]map[string]any)
	p, pok := mx["p"].(interface{ String() string })
	ps := ""
	if pok {
		ps = p.String()
	}
	set("mx", ok, rok, roles, mok, mm, nok, n, pok, ps)
	tm, ok := ctx.Value("time").(time.Time)
	set("time", ok, tm.Year(), tm.Second())
	d, ok := ctx.Value("d").(time.Duration)
	set("d", ok, int64(d))
	u, ok := ctx.Value("url").(*url.URL)
	if ok {
		set("url", ok, u.Path, u.Query().Get("q"))
	} else {
		set("url", ok)
	}
	ip, ok := ctx.Value("ip").(net.IP)
	set("ip", ok, ip.String())
	ck, ok := ctx.Value("cookie").(*http.Cookie)
	if ok {
		set("cookie", ok, ck.Value)
	} else {
		set("cookie", ok)
	}
	num, ok := ctx.Value("num").(json.Number)
	set("num", ok, string(num))
	hd, ok := ctx.Value("hdr").(http.Header)
	set("hdr", ok, hd.Get("K"))
	vals, ok := ctx.Value("vals").(url.Values)
	set("vals", ok, vals.Get("q"))
	req, ok := ctx.Value("req").(*http.Request)
	if ok {
		set("req", ok, req.Method)
	} else {
		set("req", ok)
	}
	sm, ok := ctx.Value("smap").(*sync.Map)
	if ok {
		v, _ := sm.Load("k")
		set("smap", ok, v)
	} else {
		set("smap", ok)
	}
	av, ok := ctx.Value("aval").(*atomic.Value)
	if ok {
		set("aval", ok, av.Load())
	} else {
		set("aval", ok)
	}
	buf, ok := ctx.Value("buf").(*bytes.Buffer)
	if ok {
		set("buf", ok, buf.String())
	} else {
		set("buf", ok)
	}
	sb, ok := ctx.Value("sb").(*strings.Builder)
	if ok {
		set("sb", ok, sb.String())
	} else {
		set("sb", ok)
	}
	raw, ok := ctx.Value("raw").(json.RawMessage)
	set("raw", ok, string(raw))
}`
