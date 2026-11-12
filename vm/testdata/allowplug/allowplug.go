// Package allowplug lets a request through when its client address is in
// an allow list, compared with == on netip.Addr and netip.Prefix, like the
// allow list of tomMoulard/fail2ban. Other clients get
// 429.
package allowplug

import (
	"context"
	"net/http"
	"net/netip"
)

type Config struct {
	Allow []string
}

func CreateConfig() *Config { return &Config{Allow: []string{"192.0.2.1"}} }

type allow struct {
	next   http.Handler
	addrs  []netip.Addr
	prefix netip.Prefix
}

func New(_ context.Context, next http.Handler, cfg *Config, _ string) (http.Handler, error) {
	a := &allow{next: next, prefix: netip.MustParsePrefix("10.0.0.0/8")}
	for _, s := range cfg.Allow {
		ip, err := netip.ParseAddr(s)
		if err != nil {
			return nil, err
		}
		a.addrs = append(a.addrs, ip)
	}
	return a, nil
}

func (a *allow) allowed(ip netip.Addr) bool {
	for _, x := range a.addrs {
		if x == ip {
			return true
		}
	}
	p, _ := ip.Prefix(8)
	return p == a.prefix
}

func (a *allow) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil || !a.allowed(ap.Addr()) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	a.next.ServeHTTP(w, r)
}
