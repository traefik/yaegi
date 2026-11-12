// The standard library with interpreted values: sort.Sort with an
// interpreted sort.Interface, sort.Slice and sort.Stable, slices and maps
// functions with interpreted types and funcs, container/heap, regexp,
// time (durations, formatting, fixed zones), context values with
// interpreted keys and values, strings.NewReader with io helpers,
// bufio.Scanner with an interpreted split func, and text/tabwriter.
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"container/heap"
	"context"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

type Person struct {
	Name string
	Age  int
}

type ByAge []Person

func (a ByAge) Len() int           { return len(a) }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

type ctxKey struct{ name string }
type otherKey string

type upperWriter struct{ w io.Writer }

func (u upperWriter) Write(p []byte) (int, error) {
	return u.w.Write(bytes.ToUpper(p))
}

type countReader struct {
	r io.Reader
	n int
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func main() {
	people := []Person{{"alice", 30}, {"bob", 25}, {"carol", 35}, {"dave", 25}}
	sort.Sort(ByAge(people))
	fmt.Println(people)
	sort.Sort(sort.Reverse(ByAge(people)))
	fmt.Println(people)
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	sort.Stable(ByAge(people))
	fmt.Println(people, sort.IsSorted(ByAge(people)))
	i := sort.Search(len(people), func(i int) bool { return people[i].Age >= 30 })
	fmt.Println(i)

	slices.SortFunc(people, func(a, b Person) int {
		return cmp.Or(cmp.Compare(b.Age, a.Age), strings.Compare(a.Name, b.Name))
	})
	fmt.Println(people)
	idx := slices.IndexFunc(people, func(p Person) bool { return p.Name == "bob" })
	fmt.Println(idx, slices.Contains(people, Person{"dave", 25}), slices.Index(people, Person{"x", 1}))
	people = slices.DeleteFunc(people, func(p Person) bool { return p.Age == 25 })
	fmt.Println(people, len(people))
	people = slices.Insert(people, 1, Person{"eve", 40})
	fmt.Println(people, slices.Compact([]int{1, 1, 2, 2, 2, 3, 1}))
	groups := map[int][]string{}
	for _, p := range []Person{{"a", 1}, {"b", 2}, {"c", 1}} {
		groups[p.Age] = append(groups[p.Age], p.Name)
	}
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		fmt.Print(k, groups[k], " ")
	}
	fmt.Println()
	m2 := maps.Clone(groups)
	m2[1][0] = "changed" // shares the slices
	delete(m2, 2)
	fmt.Println(groups[1], len(groups), len(m2), maps.EqualFunc(groups, groups, slices.Equal))

	h := &IntHeap{5, 2, 8}
	heap.Init(h)
	heap.Push(h, 3)
	heap.Push(h, 1)
	var popped []int
	for h.Len() > 0 {
		popped = append(popped, heap.Pop(h).(int))
	}
	fmt.Println(popped)

	re := regexp.MustCompile(`(?P<key>\w+)=(?P<val>[^;]*)`)
	s := "a=1;bb=two;c="
	fmt.Println(re.FindAllStringSubmatch(s, -1))
	fmt.Println(re.ReplaceAllString(s, "${val}:${key}"), re.SubexpNames())
	fmt.Println(re.ReplaceAllStringFunc(s, strings.ToUpper))
	fmt.Println(re.FindStringIndex(s), re.MatchString("nope"), re.NumSubexp())
	_, err := regexp.Compile(`(`)
	fmt.Println(err)

	d := 90*time.Minute + 30*time.Second + 5*time.Millisecond
	fmt.Println(d, d.Hours(), d.Round(time.Minute), d.Truncate(time.Hour), time.Duration(0), -d)
	pd, err := time.ParseDuration("1h2m3.5s")
	fmt.Println(pd, err)
	zone := time.FixedZone("X", 2*3600)
	t := time.Date(2024, time.February, 29, 13, 4, 5, 6000, zone)
	fmt.Println(t, t.UTC().Format(time.RFC3339Nano), t.Weekday(), t.YearDay(), t.AddDate(1, 0, 0).Format("2006-01-02"))
	fmt.Println(t.Format("Mon Jan _2 15:04:05.000 MST 2006"), t.Unix(), t.Add(-d).Sub(t))
	pt, err := time.Parse("2006-01-02 15:04", "2023-10-01 08:30")
	fmt.Println(pt, err, pt.Before(t), t.Truncate(24*time.Hour).Equal(t))
	_, err = time.Parse(time.RFC3339, "bad")
	fmt.Println(err)

	ctx := context.WithValue(context.Background(), ctxKey{"user"}, Person{"zoe", 9})
	ctx = context.WithValue(ctx, otherKey("user"), "string key")
	ctx = context.WithValue(ctx, "user", 42)
	fmt.Println(ctx.Value(ctxKey{"user"}), ctx.Value(otherKey("user")), ctx.Value("user"), ctx.Value(ctxKey{"nobody"}))
	cctx, cancel := context.WithCancelCause(ctx)
	cancel(fmt.Errorf("stopped by %s", "test"))
	fmt.Println(cctx.Err(), context.Cause(cctx), cctx.Value(ctxKey{"user"}).(Person).Name)
	wctx := context.WithoutCancel(cctx)
	fmt.Println(wctx.Err(), wctx.Value("user"))
	stop := context.AfterFunc(ctx, func() {})
	fmt.Println(stop())

	cr := &countReader{r: strings.NewReader("line one\nline two\n\nlast")}
	sc := bufio.NewScanner(cr)
	sc.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		adv, tok, err := bufio.ScanLines(data, atEOF)
		if tok != nil {
			tok = bytes.TrimSpace(tok)
		}
		return adv, tok, err
	})
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	fmt.Printf("%q %d %v\n", lines, cr.n, sc.Err())

	var out bytes.Buffer
	uw := upperWriter{&out}
	n, err := io.Copy(uw, io.LimitReader(strings.NewReader("copy me please"), 7))
	fmt.Println(n, err, out.String())
	mr := io.MultiReader(strings.NewReader("ab"), strings.NewReader("cd"))
	var sb strings.Builder
	tee := io.TeeReader(mr, &sb)
	all, _ := io.ReadAll(tee)
	fmt.Println(string(all), sb.String())
	pr, pw := io.Pipe()
	go func() {
		fmt.Fprint(pw, "through a pipe")
		pw.Close()
	}()
	pb, _ := io.ReadAll(pr)
	fmt.Println(string(pb))

	tw := tabwriter.NewWriter(&out, 0, 8, 1, ' ', 0)
	out.Reset()
	fmt.Fprintln(tw, "name\tage\t")
	for _, p := range []Person{{"al", 1}, {"barbara", 22}} {
		fmt.Fprintf(tw, "%s\t%d\t\n", p.Name, p.Age)
	}
	tw.Flush()
	fmt.Print(out.String())
}
