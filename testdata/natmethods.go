package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Calls of native functions and methods with simple signatures take a
// generated direct call (vm/zz_natsigs.go). The receiver can be a native
// pointer or value, a native value in a variable or a field of interpreted
// code, or a named type with a basic underlying type (time.Duration). Any
// other form, a nil pointer or an interpreted form of the type, takes the
// general path. The results must be the ones of Go in every case.

type holder struct {
	b  strings.Builder
	t  time.Time
	u  url.URL
	re *regexp.Regexp
}

func main() {
	// regexp: native pointer receiver.
	re := regexp.MustCompile(`^/api/v(\d+)/users/(?P<id>\d+)(/.*)?$`)
	fmt.Println(re.FindStringSubmatch("/api/v2/users/123/p"))
	fmt.Println(re.MatchString("/api/v2/users/x"), re.MatchString("/api/v9/users/1"))
	fmt.Println(re.NumSubexp(), re.SubexpNames(), re.SubexpIndex("id"), re.String())
	fmt.Println(re.ReplaceAllString("/api/v1/users/7/a", "v=$1 id=${id}"))
	fmt.Println(regexp.MustCompile(`a+`).FindAllString("caaab a aa", -1))
	fmt.Println(regexp.QuoteMeta("a.b*c"))

	// time: named basic types and struct values.
	d := 90*time.Minute + 30*time.Second
	fmt.Println(d.String(), d.Hours(), d.Minutes(), d.Round(time.Hour), d.Truncate(time.Hour))
	fmt.Println(time.March.String(), time.Saturday.String(), time.Duration(1500).String())
	t := time.Date(2024, time.February, 29, 13, 4, 5, 6, time.UTC)
	fmt.Println(t.Unix(), t.Year(), t.YearDay(), t.Format(time.RFC3339Nano), t.Month(), t.Weekday())
	u2 := t.Add(36 * time.Hour)
	fmt.Println(u2.Format("2006-01-02 15:04"), t.Before(u2), u2.Before(t), t.Equal(t), u2.Sub(t))
	fmt.Println(t.Truncate(time.Hour).Format(time.Kitchen), t.IsZero(), time.Time{}.IsZero())
	pd, err := time.ParseDuration("1h2m3.5s")
	fmt.Println(pd, err)
	_, err = time.ParseDuration("bad")
	fmt.Println(err)

	// net/url: pointer receivers, a pointer argument and pointer results.
	u, err := url.Parse("https://user:pw@example.com:8443/a/b%20c?x=1&y=2#frag")
	if err != nil {
		panic(err)
	}
	fmt.Println(u.Hostname(), u.Port(), u.EscapedPath(), u.IsAbs(), u.Redacted(), u.RequestURI())
	ref, _ := url.Parse("../d?z=3")
	fmt.Println(u.ResolveReference(ref).String(), u.Query().Get("y"))
	fmt.Println(url.QueryEscape("a b&c"), url.PathEscape("a b/c"))
	_, err = url.Parse("http://[::1")
	fmt.Println(err)
	// A url.URL value of interpreted code: a variable and a field.
	lu := url.URL{Scheme: "http", Host: "h:80", Path: "/p q"}
	fmt.Println(lu.String(), lu.Port(), lu.EscapedPath())
	var h holder
	h.u = url.URL{Scheme: "https", Host: "f", Path: "/x"}
	fmt.Println(h.u.String(), h.u.Hostname())

	// strings.Builder and bytes.Buffer: a local variable, a field, a pointer.
	var b strings.Builder
	b.WriteString("abc")
	b.WriteByte('-')
	b.WriteRune('é')
	b.Grow(10)
	fmt.Println(b.String(), b.Len())
	h.b.WriteString("field")
	pb := &h.b
	pb.WriteString("+ptr")
	fmt.Println(h.b.String(), pb.Len())
	var buf bytes.Buffer
	buf.WriteString("line1\nline2\n")
	line, err := buf.ReadString('\n')
	fmt.Printf("%q %v %d %q\n", line, err, buf.Len(), buf.String())
	nb := new(bytes.Buffer)
	nb.WriteString("x")
	fmt.Println(nb.String(), nb.Cap() >= 1)

	// bufio over a strings.Reader.
	br := bufio.NewReader(strings.NewReader("first line\nsecond"))
	l1, err := br.ReadString('\n')
	fmt.Printf("%q %v %d\n", l1, err, br.Buffered())
	r, size, err := br.ReadRune()
	fmt.Println(string(r), size, err)

	// net/http: request methods, functions with basic types.
	req := httptest.NewRequest("GET", "http://example.com/p?q=v", nil)
	req.Header.Set("User-Agent", "agent/1")
	req.Header.Set("Referer", "http://r/")
	req.AddCookie(&http.Cookie{Name: "n", Value: "c"})
	fmt.Println(req.UserAgent(), req.Referer(), req.FormValue("q"), req.ProtoAtLeast(1, 1), req.ProtoAtLeast(2, 0))
	ck, err := req.Cookie("n")
	fmt.Println(ck.Value, err)
	_, err = req.Cookie("missing")
	fmt.Println(err)
	fmt.Println(http.StatusText(404), http.CanonicalHeaderKey("x-foo-bar"))

	// strconv, net and netip.
	n, err := strconv.ParseInt("-ff", 16, 64)
	fmt.Println(n, err)
	_, err = strconv.ParseInt("999999999999999999999", 10, 64)
	fmt.Println(err)
	fmt.Println(strconv.FormatFloat(3.14159, 'f', 2, 64), strconv.Quote("a\"b"), strconv.QuoteRune('☺'))
	bv, err := strconv.ParseBool("true")
	fmt.Println(bv, err)
	host, port, err := net.SplitHostPort("[::1]:80")
	fmt.Println(host, port, err, net.JoinHostPort("::1", "443"))
	a, err := netip.ParseAddr("192.168.1.10")
	fmt.Println(a, err, a.Is4(), a.IsPrivate(), a.String())
	pfx := netip.MustParsePrefix("192.168.0.0/16")
	fmt.Println(pfx.Contains(a), pfx.Bits(), pfx.Masked())

	// A nil pointer receiver panics as in Go.
	func() {
		defer func() { fmt.Println("recovered:", recover() != nil) }()
		var nu *url.URL
		fmt.Println(nu.String())
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover() != nil) }()
		h.re.String()
	}()
}
