package main

// The scenarios of the catalog harness beyond the default middleware one
// (compare/catalog/README.md, "Other scenarios"):
//
//   - auth: the credential plugins get a configuration that trusts local keys
//     (scenarios/auth.yml), and requests with valid and invalid credentials:
//     JWTs signed here, a local OIDC provider, basic auth and API keys;
//   - reload: CreateConfig, decoding and New repeated with configurations that
//     change, then requests; goroutines and heap are compared with native;
//   - provider: the provider plugins (scenarios/providers.yml) run as Traefik
//     runs them, against local mocks of the APIs they poll.
//
// The local OIDC provider, the JWKS and the mocks are served by this process
// on -authaddr, the same address for every host, so that the transcripts can
// be compared.

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	scenariosFlag = flag.String("scenarios", strings.Join(allScenarios, ","), "scenarios to run (comma separated): "+strings.Join(allScenarios, ", "))
	authAddr      = flag.String("authaddr", "127.0.0.1:38551", "address of the local OIDC provider, JWKS and API mocks")
	reloadRounds  = flag.Int("reloads", 200, "reload scenario: rounds of CreateConfig + decode + New")
)

// allScenarios are the values of -scenarios: middleware (main.go), auth,
// reload, reloadcancel, reloadsandbox and provider (this file), headers,
// stream and conc (extra.go). The hosts take the same names with -scenario
// (vmhost/driver*.go), except reloadcancel and reloadsandbox, which are the
// reload scenario of the hosts with -reloadctx cancel (and -sandbox for the
// yaegi v2 hosts).
var allScenarios = []string{"middleware", "auth", "reload", "reloadcancel", "reloadsandbox", "provider", "headers", "stream", "conc"}

// reloadScenarios are the scenarios of runReload (vmhost/driver_scenarios.go).
var reloadScenarios = []string{"reload", "reloadcancel", "reloadsandbox"}

func isReload(scen string) bool { return slices.Contains(reloadScenarios, scen) }

// runs tells whether scenario s was selected with -scenarios.
func runs(s string) bool {
	for _, x := range strings.Split(*scenariosFlag, ",") {
		if strings.TrimSpace(x) == s {
			return true
		}
	}
	return false
}

// checkScenarios rejects the unknown names of -scenarios.
func checkScenarios() error {
	for _, x := range strings.Split(*scenariosFlag, ",") {
		x = strings.TrimSpace(x)
		known := x == ""
		for _, s := range allScenarios {
			known = known || x == s
		}
		if !known {
			return fmt.Errorf("unknown scenario %q in -scenarios (known: %s)", x, strings.Join(allScenarios, ", "))
		}
	}
	return nil
}

// engineOf returns the engine of a host mode: "auth-govm" is "govm".
func engineOf(mode string) string {
	if i := strings.LastIndexByte(mode, '-'); i >= 0 {
		return mode[i+1:]
	}
	return mode
}

// ---- data files ----

// authEntry is one plugin of scenarios/auth.yml.
type authEntry struct {
	Repo  string `yaml:"repo"`
	Class string `yaml:"class"` // jwt, oidc, basic, apikey, forward-auth, ldap, oauth...
	// Skip says why the plugin cannot be configured locally: it is still
	// listed in the results.
	Skip string `yaml:"skip"`
	Note string `yaml:"note"`
	// Config replaces the manifest testData (placeholders expanded).
	Config map[string]any `yaml:"config"`
	// Token generates one request per JWT variant of Alg, sent as Header
	// (with Prefix), Cookie or Query, plus one request without a token.
	Token *struct {
		Alg    string `yaml:"alg"` // RS256 or HS256
		Header string `yaml:"header"`
		Prefix string `yaml:"prefix"`
		Cookie string `yaml:"cookie"`
		Query  string `yaml:"query"`
		Target string `yaml:"target"`
	} `yaml:"token"`
	Requests []reqSpec `yaml:"requests"`
	Mocks    []mock    `yaml:"mocks"`
}

type reqSpec struct {
	Name    string      `yaml:"name" json:"name"`
	Method  string      `yaml:"method" json:"method,omitempty"`
	Target  string      `yaml:"target" json:"target"`
	Remote  string      `yaml:"remote" json:"remote,omitempty"`
	Body    string      `yaml:"body" json:"body,omitempty"`
	Header  [][2]string `yaml:"header" json:"header,omitempty"`
	Cookies bool        `yaml:"cookies" json:"cookies,omitempty"`
}

// mock is one answer of the local API server. When Require is set, the
// request must carry these headers (exact values), else the next mock of the
// same path is tried, and at the end the server answers 401.
type mock struct {
	Path    string            `yaml:"path"`
	Method  string            `yaml:"method"`
	Status  int               `yaml:"status"`
	Header  map[string]string `yaml:"header"`
	Body    string            `yaml:"body"`
	Require [][2]string       `yaml:"require"`
	// Contains is a text that the request body must contain.
	Contains string `yaml:"contains"`
}

// providerEntry is one plugin of scenarios/providers.yml.
type providerEntry struct {
	Repo   string         `yaml:"repo"`
	Skip   string         `yaml:"skip"`
	Note   string         `yaml:"note"`
	Config map[string]any `yaml:"config"`
	Mocks  []mock         `yaml:"mocks"`
	// Coarse is why the emitted configurations cannot be compared (they
	// depend on the clock): only the stages and the number of messages are.
	Coarse string `yaml:"coarse"`
}

var (
	authData     = map[string]*authEntry{}
	providerData = map[string]*providerEntry{}
	mocks        = map[string][]mock{}
)

func readData(name string, v any) {
	b, err := os.ReadFile(filepath.Join(root, "scenarios", name))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		os.Exit(1)
	}
}

func wantsScenarios(p *plugin) bool {
	return p.scope == "provider" && runs("provider")
}

// ---- keys, tokens and placeholders ----

var (
	rsaKey, otherKey *rsa.PrivateKey
	rsaPubPEM        string
	rsaCertPEM       string
	tokens           map[string]string
)

const (
	hmacSecret = "catalog-hmac-secret-0123456789abcdef"
	clientID   = "catalog-client"
	rsaKid     = "catalog-rsa"
)

func issuer() string { return "http://" + *authAddr }

func readKey(name string) *rsa.PrivateKey {
	b, err := os.ReadFile(filepath.Join(root, "scenarios", name))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	blk, _ := pem.Decode(b)
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		os.Exit(1)
	}
	return k.(*rsa.PrivateKey)
}

// baseClaims are the claims of a valid token. The dates are fixed (and RS256
// signatures deterministic) so that the tokens are the same in every run.
func baseClaims() map[string]any {
	return map[string]any{
		"iss": issuer(), "aud": clientID, "azp": clientID, "sub": "user-42",
		"email": "user42@example.com", "email_verified": true, "name": "User 42",
		"preferred_username": "user42", "given_name": "User", "family_name": "Fortytwo",
		"groups": []string{"admins", "dev"}, "roles": []string{"admin"},
		"realm_access": map[string]any{"roles": []string{"admin", "user"}},
		"scope":        "openid profile email",
		"org":          map[string]any{"id": 7, "name": "acme"},
		"exp":          int64(4102444800), // 2100-01-01; int64: past the 32-bit int range
		"iat":          1700000000,        // 2023-11-14
		"nbf":          1700000000,
		"jti":          "catalog-jti-1",
		// The claims of the GitHub OAuth plugins (MuXiu1997, luizfonseca).
		"id": "996", "login": "user42", "teams": []string{"876255"},
	}
}

// makeJWT signs claims with alg (RS256 with key, HS256 with secret, none).
func makeJWT(alg string, claims map[string]any, key *rsa.PrivateKey, secret string) string {
	h := map[string]any{"alg": alg, "typ": "JWT"}
	if alg == "RS256" {
		h["kid"] = rsaKid
	}
	hb, _ := json.Marshal(h)
	cb, _ := json.Marshal(claims)
	in := b64(hb) + "." + b64(cb)
	var sig []byte
	switch alg {
	case "RS256":
		d := sha256.Sum256([]byte(in))
		sig, _ = rsa.SignPKCS1v15(nil, key, crypto.SHA256, d[:])
	case "HS256":
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(in))
		sig = m.Sum(nil)
	}
	return in + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// makeTokens builds the token variants, for RS256 and HS256: valid, expired,
// badsig (signed with another key or secret), minimal (iss, aud, exp and iat
// only), floatexp (NumericDate with a fraction, as golang-jwt accepts),
// wrongiss, wrongaud, notyet (nbf in the future); plus none (alg none) and
// garbage.
func makeTokens() map[string]string {
	t := map[string]string{"none": makeJWT("none", baseClaims(), nil, ""), "garbage": "not.a.jwt"}
	for _, alg := range []string{"RS256", "HS256"} {
		a := strings.ToLower(alg)
		sign := func(c map[string]any) string { return makeJWT(alg, c, rsaKey, hmacSecret) }
		with := func(k string, v any) map[string]any { c := baseClaims(); c[k] = v; return c }
		t[a+"-valid"] = sign(baseClaims())
		t[a+"-expired"] = sign(with("exp", 1700003600))
		t[a+"-badsig"] = makeJWT(alg, baseClaims(), otherKey, hmacSecret+"-wrong")
		t[a+"-minimal"] = sign(map[string]any{"iss": issuer(), "aud": clientID, "exp": int64(4102444800), "iat": 1700000000})
		t[a+"-floatexp"] = sign(with("exp", 4102444800.5))
		t[a+"-wrongiss"] = sign(with("iss", "https://issuer.example.org"))
		t[a+"-wrongaud"] = sign(with("aud", "another-client"))
		t[a+"-notyet"] = sign(with("nbf", int64(4000000000)))
		t[a+"-fresh"] = sign(fresh(baseClaims()))
	}
	return t
}

// startTime is the start of the run, to the minute: the "fresh" tokens, for
// the plugins that bound the age of iat, are the same for every host of a
// run but change between runs.
var startTime = time.Now().Truncate(time.Minute).Unix()

// fresh sets iat and nbf a minute before the start of the run and exp an
// hour after.
func fresh(c map[string]any) map[string]any {
	c["iat"], c["nbf"], c["exp"] = startTime-60, startTime-60, startTime+3600
	return c
}

var tokenVariants = []string{"valid", "expired", "badsig", "minimal", "floatexp", "wrongiss", "wrongaud", "notyet", "fresh"}

var placeholder = regexp.MustCompile(`\{\{([a-z0-9_]+)(?::([^}]*))?\}\}`)

// expand replaces the placeholders of the data files. The flow placeholders
// ({{state}}, {{code}}, {{redirect_uri}}) are left to the driver.
func expand(s string) string {
	host, port, _ := net.SplitHostPort(*authAddr)
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		sm := placeholder.FindStringSubmatch(m)
		switch sm[1] {
		case "rsa_public_pem":
			return rsaPubPEM
		case "rsa_cert_pem":
			return rsaCertPEM
		case "hmac_secret":
			return hmacSecret
		case "issuer", "backend":
			return issuer()
		case "backend_host":
			return host
		case "backend_port":
			return port
		case "jwks_url":
			return issuer() + "/jwks"
		case "token":
			if v, ok := tokens[sm[2]]; ok {
				return v
			}
			fmt.Fprintln(os.Stderr, "unknown token", sm[2])
			os.Exit(1)
		case "basic":
			return base64.StdEncoding.EncodeToString([]byte(sm[2]))
		case "b64":
			return base64.StdEncoding.EncodeToString([]byte(expand("{{" + sm[2] + "}}")))
		}
		return m
	})
}

// expandAny expands the placeholders in every string of v.
func expandAny(v any) any {
	switch x := v.(type) {
	case string:
		return expand(x)
	case map[string]any:
		m := map[string]any{}
		for k, e := range x {
			m[k] = expandAny(e)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, e := range x {
			l[i] = expandAny(e)
		}
		return l
	}
	return v
}

// ---- local OIDC provider, JWKS and mocks ----

func startScenarios() {
	// Every scenario but middleware runs its hosts behind the no-network
	// proxy, the soak too (also under -reuse, which still runs the soak).
	if needsNoNet() {
		if err := startNoNetProxy(); err != nil {
			fmt.Fprintln(os.Stderr, "no-network proxy:", err)
			os.Exit(1)
		}
	}
	if !runs("auth") && !runs("provider") && !slices.ContainsFunc(reloadScenarios, runs) {
		return
	}
	var al []*authEntry
	readData("auth.yml", &al)
	for _, e := range al {
		authData[e.Repo] = e
		for _, m := range e.Mocks {
			mocks[m.Path] = append(mocks[m.Path], m)
		}
	}
	var pl []*providerEntry
	readData("providers.yml", &pl)
	for _, e := range pl {
		providerData[e.Repo] = e
		for _, m := range e.Mocks {
			mocks[m.Path] = append(mocks[m.Path], m)
		}
	}
	rsaKey, otherKey = readKey("test-rsa-key.pem"), readKey("other-rsa-key.pem")
	pb, _ := os.ReadFile(filepath.Join(root, "scenarios", "test-rsa-pub.pem"))
	cb, _ := os.ReadFile(filepath.Join(root, "scenarios", "test-rsa-cert.pem"))
	rsaPubPEM, rsaCertPEM = string(pb), string(cb)
	tokens = makeTokens()
	if *reuse {
		return
	}
	ln, err := net.Listen("tcp", *authAddr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local OIDC provider:", err)
		os.Exit(1)
	}
	go http.Serve(ln, http.HandlerFunc(serveLocal))
}

// needsNoNet tells whether a selected scenario runs hosts behind noNetProxy:
// all of them but middleware, and the soak.
func needsNoNet() bool {
	for _, s := range allScenarios {
		if s != "middleware" && runs(s) {
			return true
		}
	}
	return *soakDur > 0
}

// startNoNetProxy starts the proxy of noNetProxy.
func startNoNetProxy() error {
	pl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	noNetProxy = "http://" + pl.Addr().String()
	go http.Serve(pl, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "catalog scenarios: no network", http.StatusForbidden)
	}))
	return nil
}

// noNetProxy is the address of a proxy that refuses every request (403, also
// for CONNECT). The hosts of every scenario but middleware (auth, reload,
// provider, headers, stream, conc) and of the soak use it
// as HTTP_PROXY and HTTPS_PROXY, with NO_PROXY for the local servers: a
// plugin that downloads from the internet in New (bot-wrangler fetches a list
// from cdn.jsdelivr.net) then gets the same answer in every round and every
// engine, and the results do not depend on the network of the machine. Only
// clients that read the proxy from the environment are covered
// (http.DefaultTransport and transports with http.ProxyFromEnvironment).
var noNetProxy string

// noNetEnv returns the environment that sends the HTTP clients of a host of
// mode to noNetProxy, or nil for the middleware scenario (its modes have no
// "-"; the other ones are "<scenario>-<engine>", as "conc-govm" or
// "soak-aot") and when no proxy runs.
func noNetEnv(mode string) []string {
	if noNetProxy == "" || !strings.Contains(mode, "-") {
		return nil
	}
	var env []string
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		env = append(env, k+"="+noNetProxy)
	}
	return append(env, "NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost")
}

func serveLocal(w http.ResponseWriter, r *http.Request) {
	if ms, ok := mocks[r.URL.Path]; ok {
		for _, m := range ms {
			if m.Method != "" && m.Method != r.Method {
				continue
			}
			ok := true
			for _, kv := range m.Require {
				if r.Header.Get(kv[0]) != expand(kv[1]) {
					ok = false
				}
			}
			if m.Contains != "" {
				b, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(b))
				ok = ok && strings.Contains(string(b), m.Contains)
			}
			if !ok {
				continue
			}
			for k, v := range m.Header {
				w.Header().Set(k, expand(v))
			}
			if m.Status == 0 {
				m.Status = http.StatusOK
			}
			if m.Header["Content-Type"] == "" && strings.HasPrefix(strings.TrimSpace(m.Body), "{") {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(m.Status)
			w.Write([]byte(expand(m.Body)))
			return
		}
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	iss := issuer()
	js := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	// Keycloak style paths (realms/<realm>/protocol/openid-connect/...) end
	// like the plain ones.
	p := r.URL.Path
	switch {
	case strings.HasSuffix(p, "/token/introspect"), strings.HasSuffix(p, "/introspect"):
		p = "/introspect"
	case strings.HasSuffix(p, "/token"):
		p = "/token"
	case strings.HasSuffix(p, "/certs"), strings.HasSuffix(p, "/jwks"):
		p = "/jwks"
	case strings.HasSuffix(p, "/openid-connect/auth"), strings.HasSuffix(p, "/authorize"):
		p = "/authorize"
	case strings.HasSuffix(p, "/userinfo"):
		p = "/userinfo"
	}
	switch p {
	case "/.well-known/openid-configuration", "/.well-known/oauth-authorization-server":
		js(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
			"userinfo_endpoint": iss + "/userinfo", "jwks_uri": iss + "/jwks", "end_session_endpoint": iss + "/logout",
			"introspection_endpoint": iss + "/introspect", "revocation_endpoint": iss + "/revoke",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256", "plain"},
			"claims_supported":                      []string{"sub", "email", "name", "preferred_username", "groups"},
		})
	case "/jwks":
		js(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": rsaKid, "use": "sig", "alg": "RS256",
			"n": b64(rsaKey.N.Bytes()), "e": b64(big.NewInt(int64(rsaKey.E)).Bytes()),
		}}})
	case "/authorize":
		q := r.URL.Query()
		u := q.Get("redirect_uri") + "?code=" + url.QueryEscape("c."+b64([]byte(q.Get("nonce")))) + "&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, u, http.StatusFound)
	case "/token":
		r.ParseForm()
		if r.Form.Get("grant_type") == "password" &&
			(r.Form.Get("username") != "user42" || r.Form.Get("password") != "catalog-password") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		nonce := ""
		if c, ok := strings.CutPrefix(r.Form.Get("code"), "c."); ok {
			b, _ := base64.RawURLEncoding.DecodeString(c)
			nonce = string(b)
		}
		// The tokens of a code flow are fresh (some plugins bound the age
		// of iat); only the ID token has the nonce (an access token with a
		// nonce passes for an ID token in some plugins).
		at := fresh(baseClaims())
		id := fresh(baseClaims())
		if nonce != "" {
			id["nonce"] = nonce
		}
		js(map[string]any{
			"access_token": makeJWT("RS256", at, rsaKey, ""), "id_token": makeJWT("RS256", id, rsaKey, ""),
			"refresh_token": "catalog-refresh-token", "token_type": "Bearer", "expires_in": 3600,
			"scope": "openid profile email",
		})
	case "/userinfo":
		c := baseClaims()
		js(map[string]any{"sub": c["sub"], "email": c["email"], "email_verified": true, "name": c["name"],
			"preferred_username": c["preferred_username"], "groups": c["groups"]})
	case "/introspect":
		r.ParseForm()
		c, ok := verifyRS256(r.Form.Get("token"))
		if !ok {
			js(map[string]any{"active": false})
			return
		}
		c["active"] = true
		js(c)
	default:
		http.NotFound(w, r)
	}
}

// verifyRS256 returns the claims of a token signed with the test key, not
// expired: what an introspection endpoint checks.
func verifyRS256(tok string) (map[string]any, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, false
	}
	d := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&rsaKey.PublicKey, crypto.SHA256, d[:], sig) != nil {
		return nil, false
	}
	cb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var c map[string]any
	if json.Unmarshal(cb, &c) != nil {
		return nil, false
	}
	if exp, ok := c["exp"].(float64); !ok || int64(exp) < time.Now().Unix() {
		return nil, false
	}
	return c, true
}

// ---- running the scenarios ----

// scenResult is the result of one scenario for one plugin.
type scenResult struct {
	cmp           string  // scenario name for the comparison (provider-coarse: see providers.yml)
	q             *plugin // res holds the outcomes per engine (native, native2, yaegi, govm, aot)
	class, classA string
	times         map[string]map[string]string
	note          string
}

var (
	scenMu  sync.Mutex
	results = map[string]map[string]*scenResult{} // scenario -> repo
)

func setResult(scen, repo string, r *scenResult) {
	scenMu.Lock()
	defer scenMu.Unlock()
	if results[scen] == nil {
		results[scen] = map[string]*scenResult{}
	}
	results[scen][repo] = r
}

func measureScenarios(p *plugin) {
	out := filepath.Join(root, "out", p.name)
	os.MkdirAll(out, 0o755)
	if p.scope == "provider" {
		e := providerData[p.repo]
		if e == nil {
			setResult("provider", p.repo, &scenResult{note: "not in scenarios/providers.yml"})
			return
		}
		if e.Skip != "" {
			setResult("provider", p.repo, &scenResult{note: "skip: " + e.Skip})
			return
		}
		td := writeJSON(out, "provider.testdata.json", expandAny(jsonable(e.Config)))
		cmp := "provider"
		if e.Coarse != "" {
			cmp = "provider-coarse"
		}
		r := runEngines(p, out, "provider", cmp, []string{"-scenario", "provider", "-testdata", td})
		r.note = e.Note
		if e.Coarse != "" {
			r.note += "; compared coarsely: " + e.Coarse
		}
		setResult("provider", p.repo, r)
		logScenario(p, "provider", r)
		return
	}
	a := authData[p.repo]
	if runs("auth") && a != nil {
		if a.Skip != "" {
			setResult("auth", p.repo, &scenResult{note: "skip: " + a.Skip})
		} else {
			td := writeJSON(out, "auth.testdata.json", expandAny(jsonable(a.Config)))
			rq := writeJSON(out, "auth.requests.json", authRequests(a))
			r := runEngines(p, out, "auth", "auth", []string{"-scenario", "auth", "-testdata", td, "-requests", rq})
			r.note = a.Note
			setResult("auth", p.repo, r)
			logScenario(p, "auth", r)
		}
	}
	if slices.ContainsFunc(reloadScenarios, runs) {
		// The configurations used in turn: the manifest testData, the
		// defaults of CreateConfig alone, and the auth configuration when
		// the plugin has one.
		cfgs := []any{jsonable(p.m.TestData), map[string]any{}}
		if a != nil && a.Skip == "" {
			cfgs = append(cfgs, expandAny(jsonable(a.Config)))
		}
		td := writeJSON(out, "reload.testdata.json", jsonable(p.m.TestData))
		rc := writeJSON(out, "reload.configs.json", cfgs)
		args := []string{"-scenario", "reload", "-testdata", td, "-reloadconfigs", rc, "-reloads", strconv.Itoa(*reloadRounds)}
		for _, rs := range []struct {
			scen  string
			extra []string
			eo    engineOpts
		}{
			// One context for all the rounds.
			{"reload", nil, engineOpts{}},
			// The context of each round is canceled before the next
			// round, as Traefik does (runReload).
			{"reloadcancel", []string{"-reloadctx", "cancel"}, engineOpts{}},
			// The same with a budget and an Options.Sandbox in the yaegi v2
			// hosts: native and yaegi are the ones of reloadcancel when
			// it ran just before.
			{"reloadsandbox", []string{"-reloadctx", "cancel"}, engineOpts{vmExtra: []string{"-sandbox"}}},
		} {
			if !runs(rs.scen) {
				continue
			}
			if rs.scen == "reloadsandbox" && runs("reloadcancel") {
				rs.eo.sameAs = "reloadcancel"
			}
			r := runEngines(p, out, rs.scen, "reload", append(args[:len(args):len(args)], rs.extra...), rs.eo)
			setResult(rs.scen, p.repo, r)
			logScenario(p, rs.scen, r)
		}
	}
}

func logScenario(p *plugin, scen string, r *scenResult) {
	fmt.Fprintf(os.Stderr, "%-8s %-47s native=%-6s yaegi=%-6s govm=%-6s aot=%-6s %s | %s\n", scen, p.repo,
		r.q.res["native"].stage, sVerdict(r.q, r.cmp, "yaegi"), sVerdict(r.q, r.cmp, "govm"), sVerdict(r.q, r.cmp, "aot"), r.class, r.classA)
}

func writeJSON(out, name string, v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		b = []byte("{}")
	}
	f := filepath.Join(out, name)
	os.WriteFile(f, b, 0o644)
	return f
}

// authRequests returns the requests of the auth scenario of a, placeholders
// expanded.
func authRequests(a *authEntry) []reqSpec {
	var rs []reqSpec
	if t := a.Token; t != nil {
		target := t.Target
		if target == "" {
			target = "http://example.com/private/page?x=1"
		}
		rs = append(rs, reqSpec{Name: "no token", Target: target})
		alg := strings.ToLower(t.Alg)
		if alg == "" {
			alg = "rs256"
		}
		vs := []string{}
		for _, v := range tokenVariants {
			vs = append(vs, alg+"-"+v)
		}
		if alg == "rs256" {
			vs = append(vs, "hs256-valid") // a key confusion attempt
		}
		for _, v := range append(vs, "none", "garbage") {
			r := reqSpec{Name: v, Target: target}
			tok := tokens[v]
			switch {
			case t.Cookie != "":
				r.Header = [][2]string{{"Cookie", t.Cookie + "=" + tok}}
			case t.Query != "":
				r.Target += "&" + t.Query + "=" + tok
			default:
				h := t.Header
				if h == "" {
					h = "Authorization"
				}
				r.Header = [][2]string{{h, t.Prefix + tok}}
			}
			rs = append(rs, r)
		}
	}
	for _, r := range a.Requests {
		r.Target = expand(r.Target)
		if r.Target == "" {
			r.Target = "http://example.com/private/page?x=1"
		}
		r.Body = expand(r.Body)
		h := make([][2]string, len(r.Header))
		for i, kv := range r.Header {
			h[i] = [2]string{kv[0], expand(kv[1])}
		}
		r.Header = h
		rs = append(rs, r)
	}
	return rs
}

// runEngines runs scenario scen with the native build (twice), yaegi, yaegi v2
// and yaegi v2 -aot, and classifies the outcomes as the middleware scenario does.
// engineOpts are the options of runEngines that only some scenarios use.
type engineOpts struct {
	// vmExtra are more arguments of the yaegi v2 hosts (govm and aot) only.
	vmExtra []string
	// sameAs is a scenario that ran just before with the same arguments
	// for the native and yaegi hosts: their outcomes are read from its
	// files instead of running them again.
	sameAs string
}

func runEngines(p *plugin, out, scen, cmp string, extra []string, eos ...engineOpts) *scenResult {
	var eo engineOpts
	if len(eos) > 0 {
		eo = eos[0]
	}
	ensureToolchain(p)
	args := append([]string{"-import", strings.Trim(p.m.Import, `"`), "-basepkg", p.m.BasePkg}, extra...)
	if p.m.UseUnsafe {
		args = append(args, "-unsafe")
	}
	q := &plugin{repo: p.repo, name: p.name, dir: p.dir, modDir: p.modDir, m: p.m, res: map[string]outcome{}}
	r := &scenResult{cmp: cmp, q: q, times: map[string]map[string]string{}}
	nat := filepath.Join(out, "nativehost")
	// files names the outcome files of a mode: the ones of eo.sameAs for
	// the hosts that are not run again.
	files := func(m string) string {
		if eo.sameAs != "" && (m == "native" || m == "native2" || m == "yaegi") {
			return eo.sameAs + "-" + m
		}
		return scen + "-" + m
	}
	if *reuse || eo.sameAs != "" {
		if _, err := os.Stat(nat); err != nil {
			q.res["native"] = outcome{stage: "build"}
			q.res["native2"] = q.res["native"]
		} else {
			q.res["native"] = readOutcome(out, files("native"))
			q.res["native2"] = readOutcome(out, files("native2"))
		}
	} else if exe, err := buildNative(p, out); err != nil {
		q.res["native"] = outcome{stage: "build", errLine: firstLine(err.Error())}
		q.res["native2"] = q.res["native"]
	} else {
		q.res["native"] = host(q, out, scen+"-native", exe, append(args, "-dir", p.dir))
		// Session cookies and tokens often hold the time in seconds: the
		// second native run starts in another second, so that such outputs
		// are seen as nondeterministic, not as differences of an engine.
		time.Sleep(1500 * time.Millisecond)
		q.res["native2"] = host(q, out, scen+"-native2", exe, append(args, "-dir", p.dir))
	}
	if eo.sameAs != "" {
		q.res["yaegi"] = readOutcome(out, files("yaegi"))
	} else {
		q.res["yaegi"] = host(q, out, scen+"-yaegi", filepath.Join(root, "bin/yhost"), append(args, "-dir", p.dir))
	}
	vmArgs := append(append(args[:len(args):len(args)], eo.vmExtra...), "-dir", p.modDir)
	q.res["govm"] = host(q, out, scen+"-govm", filepath.Join(root, "bin/vmhost"), vmArgs)
	q.res["aot"] = host(q, out, scen+"-aot", filepath.Join(root, "bin/vmhost"), append(vmArgs[:len(vmArgs):len(vmArgs)], "-aot"))
	for _, m := range []string{"native", "native2", "yaegi", "govm", "aot"} {
		r.times[m] = readTimes(filepath.Join(out, files(m)+".times"))
	}
	r.class = sClassify(q, cmp, "govm")
	r.classA = sClassify(q, cmp, "aot")
	return r
}

func readTimes(f string) map[string]string {
	m := map[string]string{}
	b, _ := os.ReadFile(f)
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, "\t"); ok {
			m[k] = v
		}
	}
	return m
}

// The provider transcripts hold the emitted configurations: when the two
// native runs differ (a provider that emits something else depending on the
// time), only the stages and the number of messages are compared.
var messageLine = regexp.MustCompile(`(?m)^(message \d+): .*$`)

func scenCoarse(scen, t string) string {
	if strings.HasPrefix(scen, "provider") {
		return messageLine.ReplaceAllString(t, "$1")
	}
	return coarse(t)
}

// newError is the error of New in a transcript: `new: error "<message>"`,
// or `new error "<message>"` in the reload transcripts, where the driver
// removes every ": " (so "vm: vm: x" is "vm vm x" there). Group 1 is the
// message, still quoted (%q).
var newError = regexp.MustCompile(`new:? error "((?:[^"\\\n]|\\.)*)"`)

// goVMFailure matches the errors of New, after the "vm: " that
// vm.Plugin.New adds to every panic, that are failures of yaegi v2 and not
// panics of the plugin: an error of the vm package itself (budget
// exhausted, stack overflow, the stdio, callback and load-limit errors
// (*LoadError) all start with "vm"), runtime.Goexit (vm.ErrGoexit is
// returned as is: "vm: runtime.Goexit called"), the nesting limit of
// recursive types, a construct the VM does not support (*CompileError,
// "unsupported: ...") and a method refused at run time (*RefusedError, "...
// is not allowed in a plugin"; *ImportError and *UseError, refused at load
// time, end the same way).
var goVMFailure = regexp.MustCompile(`^(vm[: ]|runtime\.Goexit called|the data for the recursive type |unsupported[: ])|( is not allowed in a plugin| the plugin needs useUnsafe)$`)

// newPanicMsg reports whether msg, the message of a failed New, is a panic
// of the plugin. The protect of the driver (native, yaegi) reports a panic
// as "panic: ...". vm.Plugin.New reports every panic as "vm: <value>":
// "vm: panic: x" for panic(x) of the plugin, "vm: runtime error: ..." and
// other runtime wordings, and the message of a panic raised by a standard
// library function ("vm: regexp: Compile(...", "vm: non-positive interval
// for NewTicker", "vm: value method T.M called using nil *T pointer"). A
// list of wordings cannot cover every library message, so every "vm: "
// error is a panic, except the known yaegi v2 failures (goVMFailure). An error
// returned by New has no "vm: " (yaegi v2 returns it as is).
func newPanicMsg(msg string) bool {
	if strings.HasPrefix(msg, "panic:") || strings.HasPrefix(msg, "panic ") {
		return true
	}
	var rest string
	switch {
	case strings.HasPrefix(msg, "vm: "):
		rest = msg[len("vm: "):]
	case strings.HasPrefix(msg, "vm "): // reload transcript
		rest = msg[len("vm "):]
	default:
		return false
	}
	return !goVMFailure.MatchString(rest)
}

// sNorm is norm (main.go), with a panic of New written "new: panic". Like a
// panic during a request, only the fact is compared: the wording of a panic
// belongs to the engine.
func sNorm(t string) string {
	return newError.ReplaceAllStringFunc(norm(t), func(m string) string {
		if newPanicMsg(newError.FindStringSubmatch(m)[1]) {
			return "new: panic"
		}
		return m
	})
}

// sMatches is matches (main.go) with the coarse transcript of scen.
func sMatches(q *plugin, scen, mode string) bool {
	n, n2, o := q.res["native"], q.res["native2"], q.res[mode]
	if n.stage == "build" || n.stage == "crash" {
		return false
	}
	if sNorm(n.transcript) == sNorm(n2.transcript) && scen != "provider-coarse" {
		return sNorm(o.transcript) == sNorm(n.transcript)
	}
	return scenCoarse(scen, sNorm(o.transcript)) == scenCoarse(scen, sNorm(n.transcript))
}

func sVerdict(q *plugin, scen, mode string) string {
	if sMatches(q, scen, mode) {
		return "ok"
	}
	if q.res[mode].stage == "ok" {
		return "diff"
	}
	return q.res[mode].stage
}

// sClassify is classify (main.go) with sVerdict.
func sClassify(q *plugin, scen, mode string) string {
	y, g := q.res["yaegi"], q.res[mode]
	vy, vg := sVerdict(q, scen, "yaegi"), sVerdict(q, scen, mode)
	switch {
	case vy == "ok" && vg == "ok":
		return "same"
	case y.stage != "crash" && g.stage != "crash" && sNorm(y.transcript) == sNorm(g.transcript):
		if vy == "load" {
			return "both fail"
		}
		return "same"
	case vy == "ok" && vg != "ok":
		return "yaegi v2 fails"
	case vg == "ok" && vy != "ok":
		return "yaegi fails"
	case vg == "load" || vg == "crash":
		if vy == "load" || vy == "crash" {
			return "both fail"
		}
		return "yaegi v2 fails"
	case vy == "load" || vy == "crash":
		return "yaegi fails"
	}
	return "different"
}

// ---- reload: growth of goroutines and heap ----

// growth returns, for one engine of a reload run, the goroutines and the
// heap (bytes) added per round between the warm-up and the end.
func growth(r *scenResult, mode string) (gor, heap float64, ok bool) {
	t := r.times[mode]
	n, err1 := strconv.Atoi(t["rounds"])
	w := 20
	g0, err2 := strconv.Atoi(t["warm_goroutines"])
	g1, err3 := strconv.Atoi(t["end_goroutines"])
	h0, err4 := strconv.ParseInt(t["warm_heap"], 10, 64)
	h1, err5 := strconv.ParseInt(t["end_heap"], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || n <= w {
		return 0, 0, false
	}
	return float64(g1-g0) / float64(n-w), float64(h1-h0) / float64(n-w), true
}

// Thresholds of leak: more than 0.5 goroutine or 64 KiB of live heap per
// round, beyond what native adds per round. Native itself can grow (a plugin
// that starts a goroutine in each New): only the difference is a fault of
// the engine.
const (
	leakGoroutines = 0.5
	leakHeap       = 64 << 10
)

// leakStop is the margin of the provider scenario: goroutines still running
// one second after Stop, beyond the most that the two native runs keep. The
// most likely leak of an engine is the Provide goroutine not stopped by Stop
// (1 goroutine, maybe 2 with a timer), so there is no margin: yaegi v2 uses the
// native net/http in both modes, so its idle HTTP connections are those of
// native, and the second native run absorbs the noise of the timing.
const leakStop = 0

// stopLeak returns "goroutines" when mode keeps more than leakStop
// goroutines after Stop beyond the maximum of native and native2 (native
// alone when native2 has no times).
func stopLeak(r *scenResult, mode string) string {
	kept := func(m string) (int, bool) {
		b, err1 := strconv.Atoi(r.times[m]["goroutines_before"])
		a, err2 := strconv.Atoi(r.times[m]["goroutines_after_stop"])
		return a - b, err1 == nil && err2 == nil
	}
	kn, okn := kept("native")
	if kn2, ok2 := kept("native2"); ok2 && okn {
		kn = max(kn, kn2)
	}
	k, ok := kept(mode)
	if !ok || !okn || k-kn <= leakStop {
		return ""
	}
	return "goroutines"
}

func leak(r *scenResult, mode string) string {
	gn, hn, okn := growth(r, "native")
	g, h, ok := growth(r, mode)
	if !ok || !okn {
		return ""
	}
	var l []string
	if g-gn > leakGoroutines {
		l = append(l, "goroutines")
	}
	if h-hn > leakHeap {
		l = append(l, "heap")
	}
	return strings.Join(l, "+")
}

// ---- results ----

func writeScenarioResults(ps []*plugin) {
	rd := filepath.Join(root, "results")
	var s strings.Builder
	fmt.Fprintf(&s, "Traefik plugin catalog, other scenarios, %s\n", time.Now().Format("2006-01-02"))
	for _, scen := range []string{"auth", "reload", "reloadcancel", "reloadsandbox", "provider"} {
		if !runs(scen) {
			continue
		}
		var b strings.Builder
		b.WriteString("repo\tclass\tnote\tnative\tdepth\tyaegi\tgovm\taot\tclass\tclass_aot")
		switch {
		case isReload(scen):
			b.WriteString("\trounds_native\trounds_yaegi\trounds_govm\trounds_aot\tgoroutines_per_round(native,yaegi,govm,aot)\theap_kib_per_round(native,yaegi,govm,aot)\tleak_yaegi\tleak_govm\tleak_aot")
		case scen == "provider":
			b.WriteString("\tgoroutines_after_stop(native,yaegi,govm,aot)\tleak_yaegi\tleak_govm\tleak_aot")
		}
		b.WriteString("\tgovm_error\taot_error\tyaegi_error\n")
		count := map[string]map[string]int{"class": {}, "class_aot": {}, "status": {}}
		var repos []string
		for _, p := range ps {
			if results[scen][p.repo] != nil {
				repos = append(repos, p.repo)
			} else if scen == "auth" && authData[p.repo] != nil && authData[p.repo].Skip != "" {
				repos = append(repos, p.repo)
			}
		}
		for _, repo := range repos {
			r := results[scen][repo]
			cls := ""
			if a := authData[repo]; scen == "auth" && a != nil {
				cls = a.Class
			}
			if r == nil || r.q == nil {
				note := ""
				if r != nil {
					note = r.note
				} else {
					note = "skip: " + authData[repo].Skip
				}
				count["status"]["not configured"]++
				fmt.Fprintf(&b, "%s\t%s\t%s\n", repo, cls, strings.ReplaceAll(note, "\t", " "))
				continue
			}
			count["status"]["measured"]++
			count["class"][r.class]++
			count["class_aot"][r.classA]++
			q := r.q
			n := q.res["native"]
			depth := "complete"
			if n.stage != "ok" && n.stage != "run" {
				depth = "stops at " + n.stage
			}
			if sNorm(n.transcript) != sNorm(q.res["native2"].transcript) {
				depth += ", nondeterministic"
			}
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s", repo, cls, strings.ReplaceAll(r.note, "\t", " "), n.stage, depth,
				sVerdict(q, r.cmp, "yaegi"), sVerdict(q, r.cmp, "govm"), sVerdict(q, r.cmp, "aot"), r.class, r.classA)
			modes := []string{"native", "yaegi", "govm", "aot"}
			switch {
			case isReload(scen):
				var gs, hs []string
				for _, m := range modes {
					fmt.Fprintf(&b, "\t%s", r.times[m]["rounds"])
					if g, h, ok := growth(r, m); ok {
						gs = append(gs, fmt.Sprintf("%.2f", g))
						hs = append(hs, fmt.Sprintf("%.1f", h/1024))
					} else {
						gs, hs = append(gs, "-"), append(hs, "-")
					}
				}
				fmt.Fprintf(&b, "\t%s\t%s", strings.Join(gs, ","), strings.Join(hs, ","))
				for _, m := range modes[1:] {
					l := leak(r, m)
					if l != "" {
						count["status"]["leak "+m]++
					}
					fmt.Fprintf(&b, "\t%s", l)
				}
			case scen == "provider":
				var gs []string
				for _, m := range modes {
					gs = append(gs, r.times[m]["goroutines_before"]+"→"+r.times[m]["goroutines_after_stop"])
				}
				fmt.Fprintf(&b, "\t%s", strings.Join(gs, ","))
				for _, m := range modes[1:] {
					l := stopLeak(r, m)
					if l != "" {
						count["status"]["leak "+m]++
					}
					fmt.Fprintf(&b, "\t%s", l)
				}
			}
			errOf := func(m string) string {
				if sVerdict(q, r.cmp, m) == "ok" {
					return ""
				}
				return strings.ReplaceAll(q.res[m].errLine, "\t", " ")
			}
			fmt.Fprintf(&b, "\t%s\t%s\t%s\n", errOf("govm"), errOf("aot"), errOf("yaegi"))
		}
		os.WriteFile(filepath.Join(rd, scen+".tsv"), []byte(b.String()), 0o644)
		fmt.Fprintf(&s, "\n%s:\n", scen)
		for _, k := range []string{"status", "class", "class_aot"} {
			fmt.Fprintf(&s, "  %s:\n", k)
			var keys []string
			for c := range count[k] {
				keys = append(keys, c)
			}
			sort.Strings(keys)
			for _, c := range keys {
				fmt.Fprintf(&s, "    %-16s %d\n", c, count[k][c])
			}
		}
	}
	os.WriteFile(filepath.Join(rd, "scenarios.txt"), []byte(s.String()), 0o644)
	fmt.Print(s.String())
}

// driverStamp is a hash of what the native host is built from: the driver
// files and the templates of the native main package.
func driverStamp() string {
	h := sha256.New()
	drvs, _ := filepath.Glob(filepath.Join(root, "vmhost/driver*.go"))
	for _, d := range drvs {
		b, _ := os.ReadFile(d)
		h.Write(b)
	}
	h.Write([]byte(nativeMain + nativeProvider))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// nativeProvider is added next to nativeMain in the native host: New of a
// provider plugin, called as the Traefik wrapper calls it.
const nativeProvider = `package main

import (
	"context"
	"fmt"
	"reflect"

	plug "IMPORT"
)

func (nativeEngine) newProvider(ctx context.Context, cfg any, name string) (providerPlugin, error) {
	res := reflect.ValueOf(plug.New).Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(cfg), reflect.ValueOf(name)})
	if len(res) > 1 && res[1].Interface() != nil {
		err, ok := res[1].Interface().(error)
		if !ok {
			return nil, fmt.Errorf("invalid error type: %T", res[1].Interface())
		}
		return nil, err
	}
	p, ok := res[0].Interface().(providerPlugin)
	if !ok {
		return nil, fmt.Errorf("invalid provider type: %T", res[0].Interface())
	}
	return p, nil
}
`
