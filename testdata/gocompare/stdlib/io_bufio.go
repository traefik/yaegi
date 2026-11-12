// io and bufio with program types implementing io.Reader, io.Writer,
// io.ReaderFrom, io.WriterTo, io.ByteReader and io.Closer: io.Copy,
// CopyN, MultiWriter, MultiReader, TeeReader, LimitReader, SectionReader,
// Pipe, ReadFull, and bufio.Scanner with custom split funcs. (io.Copy
// using the WriteTo or ReadFrom method of a program type is left out: it
// differs in yaegi v2 today.)
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// upperWriter upper-cases what it writes and counts the calls.
type upperWriter struct {
	buf   bytes.Buffer
	calls int
}

func (w *upperWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.buf.Write(bytes.ToUpper(p))
}

// slowReader returns at most n bytes per call.
type slowReader struct {
	data []byte
	n    int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	k := min(r.n, len(p), len(r.data))
	copy(p, r.data[:k])
	r.data = r.data[k:]
	return k, nil
}

// failWriter fails after limit bytes.
type failWriter struct{ limit, written int }

var errFull = errors.New("writer full")

func (f *failWriter) Write(p []byte) (int, error) {
	room := f.limit - f.written
	if len(p) > room {
		f.written += room
		return room, errFull
	}
	f.written += len(p)
	return len(p), nil
}

// rot13 is a Reader wrapper.
type rot13 struct{ r io.Reader }

func (r rot13) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	for i := 0; i < n; i++ {
		c := p[i]
		switch {
		case c >= 'a' && c <= 'z':
			p[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			p[i] = 'A' + (c-'A'+13)%26
		}
	}
	return n, err
}

// counter implements io.WriterTo.
type counter struct{ n int }

func (c counter) WriteTo(w io.Writer) (int64, error) {
	var total int64
	for i := 1; i <= c.n; i++ {
		k, err := fmt.Fprintf(w, "%d;", i)
		total += int64(k)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// Read is only there to make counter a Reader: io.Copy prefers WriteTo.
func (c counter) Read(p []byte) (int, error) {
	fmt.Println("counter.Read called")
	return 0, io.EOF
}

type closer struct {
	io.Reader
	name string
}

func (c closer) Close() error { fmt.Println("close", c.name); return nil }

func main() {
	w := &upperWriter{}
	n, err := io.Copy(w, &slowReader{[]byte("hello, program reader"), 4})
	fmt.Println(n, err, w.buf.String(), w.calls)
	var b1, b2 bytes.Buffer
	mw := io.MultiWriter(&b1, w, &b2)
	fmt.Fprintf(mw, "[%s %d]", "multi", 42)
	fmt.Println(b1.String(), b2.String(), w.buf.String())
	n, err = io.Copy(io.MultiWriter(&b1, &failWriter{limit: 5}), strings.NewReader("0123456789"))
	fmt.Println(n, err, errors.Is(err, errFull))
	var tee bytes.Buffer
	tr := io.TeeReader(rot13{strings.NewReader("Uryyb Jbeyq")}, &tee)
	all, err := io.ReadAll(tr)
	fmt.Println(string(all), err, tee.String())
	mr := io.MultiReader(strings.NewReader("ab"), &slowReader{[]byte("cde"), 1}, strings.NewReader(""), rot13{strings.NewReader("nop")})
	all, err = io.ReadAll(mr)
	fmt.Println(string(all), err)
	lr := io.LimitReader(&slowReader{[]byte("limited reader"), 3}, 9)
	all, _ = io.ReadAll(lr)
	fmt.Println(string(all))
	buf := make([]byte, 6)
	k, err := io.ReadFull(&slowReader{[]byte("abcdefgh"), 2}, buf)
	fmt.Println(k, err, string(buf))
	k, err = io.ReadFull(&slowReader{[]byte("abc"), 2}, buf)
	fmt.Println(k, err)
	k, err = io.ReadAtLeast(strings.NewReader(""), buf, 1)
	fmt.Println(k, err)
	n, err = io.CopyN(w, counter{20}, 15)
	fmt.Println(n, err, w.buf.String())
	n, err = counter{3}.WriteTo(&b1)
	fmt.Println(n, err)
	sr := io.NewSectionReader(strings.NewReader("0123456789"), 2, 5)
	all, _ = io.ReadAll(sr)
	p := make([]byte, 3)
	k, err = sr.ReadAt(p, 3)
	fmt.Println(string(all), sr.Size(), k, err, string(p[:k]))
	off, err := sr.Seek(-2, io.SeekEnd)
	fmt.Println(off, err)
	k, err = io.WriteString(w, "!ws")
	fmt.Println(k, err)
	var rc io.ReadCloser = closer{strings.NewReader("x"), "c1"}
	fmt.Println(rc.Close())
	nop := io.NopCloser(rot13{strings.NewReader("a")})
	fmt.Println(nop.Close())

	// Pipe between goroutines, in a fixed order.
	pr, pw := io.Pipe()
	go func() {
		for i := 0; i < 3; i++ {
			fmt.Fprintf(pw, "line %d\n", i)
		}
		pw.CloseWithError(errors.New("producer done"))
	}()
	sc := bufio.NewScanner(pr)
	for sc.Scan() {
		fmt.Println("piped:", sc.Text())
	}
	fmt.Println("scan err:", sc.Err())

	// Scanners with standard and custom split funcs.
	text := "alpha  beta\tgamma\nδέλτα 42 x\n\nlast"
	for name, split := range map[string]bufio.SplitFunc{} {
		_, _ = name, split
	}
	splits := []struct {
		name string
		fn   bufio.SplitFunc
	}{
		{"lines", bufio.ScanLines},
		{"words", bufio.ScanWords},
		{"runes", bufio.ScanRunes},
		{"commas", func(data []byte, atEOF bool) (int, []byte, error) {
			if i := bytes.IndexByte(data, ','); i >= 0 {
				return i + 1, bytes.TrimSpace(data[:i]), nil
			}
			if atEOF && len(data) > 0 {
				return len(data), bytes.TrimSpace(data), nil
			}
			return 0, nil, nil
		}},
		{"digits", func(data []byte, atEOF bool) (int, []byte, error) {
			start := bytes.IndexFunc(data, unicode.IsDigit)
			if start < 0 {
				if atEOF {
					return len(data), nil, nil
				}
				return len(data), nil, nil
			}
			end := bytes.IndexFunc(data[start:], func(r rune) bool { return !unicode.IsDigit(r) })
			if end < 0 {
				if atEOF {
					return len(data), data[start:], nil
				}
				return start, nil, nil
			}
			return start + end, data[start : start+end], nil
		}},
		{"stop", func(data []byte, atEOF bool) (int, []byte, error) {
			adv, tok, err := bufio.ScanWords(data, atEOF)
			if string(tok) == "gamma" {
				return 0, nil, errors.New("found gamma")
			}
			if string(tok) == "beta" {
				return adv, tok, bufio.ErrFinalToken
			}
			return adv, tok, err
		}},
	}
	for _, s := range splits {
		in := text
		if s.name == "commas" {
			in = "a, b ,c,,d "
		}
		if s.name == "digits" {
			in = "a1b22c333 4444x55555"
		}
		sc := bufio.NewScanner(&slowReader{[]byte(in), 3})
		sc.Split(s.fn)
		var toks []string
		for sc.Scan() {
			toks = append(toks, sc.Text())
		}
		fmt.Printf("%s: %q err=%v\n", s.name, toks, sc.Err())
	}
	sc = bufio.NewScanner(strings.NewReader(strings.Repeat("x", 100)))
	sc.Buffer(make([]byte, 10), 20)
	fmt.Println(sc.Scan(), sc.Err())

	// bufio Reader and Writer around program types.
	br := bufio.NewReaderSize(&slowReader{[]byte("first line\nsecond\nno newline"), 5}, 16)
	for {
		line, err := br.ReadString('\n')
		fmt.Printf("%q %v\n", line, err)
		if err != nil {
			break
		}
	}
	br = bufio.NewReader(rot13{strings.NewReader("nop qrs")})
	r, size, _ := br.ReadRune()
	pk, _ := br.Peek(3)
	fmt.Println(string(r), size, string(pk), br.Buffered(), br.UnreadRune())
	bw := bufio.NewWriterSize(w, 4)
	w.calls = 0
	bw.WriteString("ab")
	bw.WriteString("cdefgh")
	bw.WriteByte('i')
	bw.WriteRune('é')
	fmt.Println(w.calls, bw.Buffered(), bw.Available())
	fmt.Println(bw.Flush(), w.calls, w.buf.String())
	fw := bufio.NewWriterSize(&failWriter{limit: 3}, 4)
	fw.WriteString("abcdef")
	fmt.Println(fw.Flush(), fw.Flush())
}
