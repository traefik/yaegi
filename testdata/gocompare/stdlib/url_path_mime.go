// net/url, path, mime, html, text/tabwriter and strconv with program
// types: url.Parse, Query, Values, String, EscapedPath, ResolveReference,
// path.Join/Clean/Match, mime types and media params, html escaping, and
// a tabwriter writing a table of program rows.
package main

import (
	"fmt"
	"html"
	"mime"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

type Endpoint struct {
	Base  string
	Route string
	Args  map[string][]string
}

func (e Endpoint) URL() (*url.URL, error) {
	u, err := url.Parse(e.Base)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath(e.Route)
	q := u.Query()
	keys := make([]string, 0, len(e.Args))
	for k := range e.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range e.Args[k] {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u, nil
}

type Row struct {
	Name  string
	Size  int64
	Ratio float64
}

func main() {
	raws := []string{
		"https://user:p%40ss@example.com:8443/a/b%2Fc/d e?x=1&y=2&x=3&empty=&flag#frag%20ment",
		"http://[::1]:80/path?q=%zz",
		"mailto:someone@example.com",
		"/relative/path?a=b",
		"//host.only/x",
		"file:///etc/hosts",
		"http://example.com/%e2%82%ac/",
		"http://x y.com/",
		"%gh&%ij",
		"",
		"HTTP://EXAMPLE.com/Path/../a/./b",
	}
	for _, raw := range raws {
		u, err := url.Parse(raw)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("%q scheme=%q opaque=%q user=%v host=%q hostname=%q port=%q path=%q rawpath=%q escaped=%q rawq=%q frag=%q abs=%v\n",
			u.String(), u.Scheme, u.Opaque, u.User, u.Host, u.Hostname(), u.Port(), u.Path, u.RawPath, u.EscapedPath(), u.RawQuery, u.Fragment, u.IsAbs())
		if u.User != nil {
			p, ok := u.User.Password()
			fmt.Println("  user:", u.User.Username(), p, ok, u.Redacted())
		}
		q, err := url.ParseQuery(u.RawQuery)
		fmt.Println("  query:", q, err, u.Query().Get("x"), u.Query().Has("flag"), u.RequestURI())
	}
	base, _ := url.Parse("http://a/b/c/d;p?q")
	for _, ref := range []string{"g", "./g", "g/", "/g", "//g", "?y", "#s", "../g", "../..", "../../../g", "g;x?y#s", ""} {
		r, _ := url.Parse(ref)
		fmt.Println(ref, "=>", base.ResolveReference(r))
	}
	e := Endpoint{"https://api.example.com/v1/", "users/42 x", map[string][]string{"sort": {"name"}, "tag": {"a b", "c&d"}, "q": {"ü/€"}}}
	u, err := e.URL()
	fmt.Println(u, err, u.Path, u.EscapedPath())
	v := url.Values{}
	v.Set("b", "2")
	v.Add("a", "1")
	v.Add("a", "x y")
	v.Del("zz")
	fmt.Println(v.Encode(), v.Get("a"), v["a"], v.Has("b"))
	fmt.Println(url.QueryEscape("a b/c?d=e&f+g"), url.PathEscape("a b/c?d"))
	for _, s := range []string{"a%20b+c", "%zz", "%e2%82%ac", "100%"} {
		q, err1 := url.QueryUnescape(s)
		p, err2 := url.PathUnescape(s)
		fmt.Printf("%q %v %q %v\n", q, err1, p, err2)
	}
	j, err := url.JoinPath("http://h/a/", "../b", "c d")
	fmt.Println(j, err)
	ub, _ := u.MarshalBinary()
	fmt.Println(string(ub))

	// path (always forward slashes).
	for _, p := range []string{"", ".", "/", "a/b/../c", "//a//b/./c/", "../../x", "/../x", "a/b/c.tar.gz", "/a/b/"} {
		d, f := path.Split(p)
		fmt.Printf("%q clean=%q base=%q dir=%q ext=%q abs=%v split=%q,%q\n", p, path.Clean(p), path.Base(p), path.Dir(p), path.Ext(p), path.IsAbs(p), d, f)
	}
	fmt.Println(path.Join("a", "", "b/", "../c", "/d"), path.Join(), path.Join("", ""))
	for _, m := range [][2]string{{"*.go", "main.go"}, {"a/*/c", "a/b/c"}, {"a/*", "a/b/c"}, {"[a-c]?", "bz"}, {"[^a]*", "abc"}, {"\\*", "*"}, {"[", "x"}} {
		ok, err := path.Match(m[0], m[1])
		fmt.Println(m, ok, err)
	}

	// mime.
	// TypeByExtension of standard extensions reads the system tables (they
	// differ between platforms): only extensions added here are looked up.
	fmt.Println(mime.AddExtensionType(".gvmx", "application/x-yaegi; charset=utf-8"), mime.AddExtensionType("gvmy", "text/plain"))
	fmt.Printf("%q %q %q\n", mime.TypeByExtension(".gvmx"), mime.TypeByExtension(".GVMX"), mime.TypeByExtension(".gvmz"))
	for _, s := range []string{`text/html; charset=UTF-8`, `multipart/form-data; boundary="--abc"; x=y`, `attachment; filename*=UTF-8''%e2%82%ac%20rates.txt`, `bad;;`, `text/plain; charset`} {
		mt, params, err := mime.ParseMediaType(s)
		fmt.Println(mt, params, err)
	}
	fmt.Println(mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8", "name": "a b.txt"}))
	fmt.Println(mime.FormatMediaType("attachment", map[string]string{"filename": "€.txt"}))
	fmt.Println(mime.QEncoding.Encode("utf-8", "Héllo"), mime.BEncoding.Encode("utf-8", "Wörld"), mime.QEncoding.Encode("utf-8", "plain"))
	dec := new(mime.WordDecoder)
	for _, w := range []string{"=?utf-8?q?H=C3=A9llo?=", "=?UTF-8?B?V8O2cmxk?=", "=?iso-8859-1?q?caf=E9?=", "=?bad?x?y?="} {
		s, err := dec.Decode(w)
		fmt.Println(s, err)
	}
	hs, err := dec.DecodeHeader("Subject: =?utf-8?q?a?= =?utf-8?q?b?= plain")
	fmt.Println(hs, err)

	// html.
	s := `<a href="x?a=1&b=2">'Tom' & "Jerry"</a>`
	esc := html.EscapeString(s)
	fmt.Println(esc, html.UnescapeString(esc) == s, html.UnescapeString("&eacute;&#x263a;&#9731;&amp;&nbsp;|&notanentity;&lt"))

	// tabwriter.
	rows := []Row{{"alpha", 12, 0.5}, {"beta-long-name", 123456, 12.25}, {"γ", -3, 100}}
	var sb strings.Builder
	tw := tabwriter.NewWriter(&sb, 0, 8, 2, ' ', tabwriter.AlignRight|tabwriter.Debug)
	fmt.Fprintln(tw, "NAME\tSIZE\tRATIO\t")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%d\t%s\t\n", r.Name, r.Size, strconv.FormatFloat(r.Ratio, 'f', 2, 64))
	}
	tw.Flush()
	tw = tabwriter.NewWriter(&sb, 5, 0, 1, '.', 0)
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%v\t%v\n", r.Name, r.Size, r.Ratio)
	}
	fmt.Fprintln(tw, "no tabs here")
	fmt.Fprintln(tw, "x\ty")
	tw.Flush()
	fmt.Print(sb.String())
}
