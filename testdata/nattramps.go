package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Native calls that take a direct trampoline since the generated ones
// accept []byte, []string, ...string, error, io.Reader, io.Writer and
// context.Context parameters, more than 4 parameters, and results of any
// type. Each call is also made with arguments that the trampoline does not
// take (interpreted implementations of the interfaces): the general path
// must then make it, with the same results as go run.

type upper struct{ w io.Writer }

func (u upper) Write(p []byte) (int, error) { return u.w.Write(bytes.ToUpper(p)) }

type slowReader struct {
	s string
	i int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p[:1], r.s[r.i:])
	r.i += n
	return n, nil
}

type myErr struct{ msg string }

func (e *myErr) Error() string { return e.msg }

func (e *myErr) Is(target error) bool { return target == io.EOF }

type names []string

func main() {
	// []string and ...string
	parts := []string{"a", "b", "c"}
	fmt.Println(strings.Join(parts, "-"), strings.Join(nil, "-"), strings.Join([]string{}, "+"))
	fields := strings.Fields(" x  y z ") // a native slice: shared
	fmt.Println(strings.Join(fields, ","))
	fmt.Println(path.Join("a", "b/", "../c"), filepath.Join(parts...), path.Join())
	r := strings.NewReplacer("a", "1", "b", "2")
	fmt.Println(r.Replace("abc"))
	s := []string{"z", "b", "a", "y"}
	sub := s[1:3]
	sort.Strings(sub) // writes into s, through the copy of the trampoline
	fmt.Println(s, sub)
	nat := strings.Split("q,w,e,a", ",")
	sort.Strings(nat)
	fmt.Println(nat)
	ns := names{"n2", "n1"}
	sort.Strings(ns)
	fmt.Println(ns)

	// []byte
	b := []byte("hello, world")
	fmt.Println(hex.EncodeToString(b), base64.StdEncoding.EncodeToString(b[:5]), utf8.Valid(b))
	fmt.Println(subtle.ConstantTimeCompare(b[:5], []byte("hello")), subtle.ConstantTimeCompare(nil, nil))
	sum := sha256.Sum256(b)
	fmt.Println(hex.EncodeToString(sum[:4]), len(sum))
	dst := make([]byte, hex.EncodedLen(3))
	hex.Encode(dst, []byte{1, 2, 255}) // native code writes into the VM slice
	fmt.Println(string(dst))
	rd := bytes.NewReader(b)
	b[0] = 'J' // the reader shares the array, as in Go
	all, err := io.ReadAll(rd)
	fmt.Println(string(all), err)

	// error
	fmt.Println(errors.Is(io.EOF, io.EOF), errors.Is(nil, io.EOF), errors.Is(fmt.Errorf("w: %w", io.EOF), io.EOF))
	fmt.Println(errors.Is(&myErr{"m"}, io.EOF), errors.Is(io.ErrUnexpectedEOF, &myErr{"m"}))
	fmt.Println(errors.Unwrap(fmt.Errorf("w: %w", io.ErrClosedPipe)) == io.ErrClosedPipe, errors.Unwrap(&myErr{"x"}))

	// io.Reader and io.Writer
	var out bytes.Buffer
	n, err := io.Copy(&out, strings.NewReader("copy"))
	fmt.Println(n, err, out.String())
	n, err = io.Copy(upper{&out}, &slowReader{s: "slow"})
	fmt.Println(n, err, out.String())
	sw := new(strings.Builder)
	k, err := io.WriteString(sw, "ws")
	fmt.Println(k, err, sw.String())
	k, err = io.WriteString(upper{sw}, "up")
	fmt.Println(k, err, sw.String())
	lim, err := io.ReadAll(io.LimitReader(strings.NewReader("limited"), 3))
	fmt.Println(string(lim), err)
	req, err := http.NewRequest("POST", "http://x/p", strings.NewReader("body"))
	fmt.Println(req.Method, req.URL.Path, req.ContentLength, err)
	req, err = http.NewRequest("GET", "http://x/q", nil)
	fmt.Println(req.Method, req.Body == nil, req.ContentLength, err)
	req, err = http.NewRequest("PUT", "http://x/r", &slowReader{s: "abc"})
	body, _ := io.ReadAll(req.Body)
	fmt.Println(req.Method, string(body), err)

	// context.Context, and results of interface and func types
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	_, has := ctx.Deadline()
	fmt.Println(has, ctx.Err())
	cancel()
	fmt.Println(ctx.Err(), errors.Is(ctx.Err(), context.Canceled))
	vctx := context.WithValue(context.Background(), "k", "v")
	fmt.Println(vctx.Value("k"))

	// more than 4 parameters, and results of other types
	d := time.Date(2026, time.October, 23, 5, 30, 0, 0, time.UTC)
	fmt.Println(d.Format(time.RFC3339), d.Weekday())
	mt, params, err := mime.ParseMediaType("text/plain; charset=utf-8")
	fmt.Println(mt, params["charset"], len(params), err)
	_, _, err = mime.ParseMediaType("")
	fmt.Println(err)
}
