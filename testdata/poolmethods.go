package main

// A pointer to an interpreted type with methods, made by an interpreted
// function that native code calls (sync.Pool's New) and given back to the
// program: it must come back with its type and identity.

import (
	"encoding/json"
	"fmt"
	"sync"
)

type Request struct {
	Headers map[string][]string `json:"1,omitempty"`
	Method  string              `json:"2,omitempty"`
}

func (r *Request) reset()                   { r.Headers = map[string][]string{}; r.Method = "" }
func (r *Request) Marshal() ([]byte, error) { return json.Marshal(r) }

var pool = sync.Pool{New: func() any { return &Request{Headers: map[string][]string{}} }}

func main() {
	r := pool.Get().(*Request)
	r.Method = "GET"
	r.Headers["X"] = []string{"1"}
	b, err := r.Marshal()
	fmt.Println(string(b), err)
	r.reset()
	pool.Put(r)
	r2, ok := pool.Get().(*Request)
	fmt.Println(ok, len(r2.Headers), r2.Method == "")
}

// session holds a channel: it has no native copy, so it crosses native code
// as an opaque handle. The guaranteed round trip goes through a sync.Map;
// the sync.Pool part only checks what Go promises (the pool may drop the
// session between Put and Get).
type session struct {
	name string
	done chan struct{}
}

func (s *session) close() { close(s.done) }

func init() {
	// A sync.Map keeps what it stores: the handle must come back as the
	// same pointer, with its methods and its channel.
	s := &session{name: "s1", done: make(chan struct{})}
	var kept sync.Map
	kept.Store("s1", s)
	v, _ := kept.Load("s1")
	got := v.(*session)
	got.close()
	_, open := <-s.done
	fmt.Println(got == s, got.name, open)

	// A sync.Pool may drop what it stores at any time (a GC between Put and
	// Get, or the goroutine moved to another P): what Get gives back is
	// either the session that was put, or a new one from New. Both are
	// *session; anything else is a value lost at the native boundary.
	made := 0
	pooled := sync.Pool{New: func() any {
		made++
		return &session{name: fmt.Sprintf("p%d", made), done: make(chan struct{})}
	}}
	p := pooled.Get().(*session) // from New: made == 1
	pooled.Put(p)
	q := pooled.Get().(*session)
	if q != p && made != 2 {
		fmt.Println("pool gave back another session without calling New:", q.name)
	}
	q.close()
	_, open = <-q.done
	fmt.Println(p.name, open, q.name == "p1" || q.name == "p2")
}
