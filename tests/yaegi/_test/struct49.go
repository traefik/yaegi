package main

type S struct {
	ts map[string][]*T
}

type T struct {
	s *S
}

func (c *S) getT(addr string) (t *T, ok bool) {
	cns, ok := c.ts[addr]
	if !ok || len(cns) == 0 {
		return nil, false
	}

	t = cns[len(cns)-1]
	c.ts[addr] = cns[:len(cns)-1]
	return t, true
}

func main() {
	s := &S{
		ts: map[string][]*T{},
	}
	s.ts["test"] = append(s.ts["test"], &T{s: s})

	t, ok := s.getT("test")
	println(t != nil, ok)
}

// Output:
// true true
