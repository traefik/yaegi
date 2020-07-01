package main

type S struct {
	ts map[string][]*T
}

func (c *S) getT(addr string) *T {
	cns, ok := c.ts[addr]
	if !ok || len(cns) == 0 {
		return nil
	}

	return cns[0]
}

type T struct {
	s *S
}

func main() {
	s := &S{
		ts: map[string][]*T{},
	}
	s.ts["test"] = append(s.ts["test"], &T{s: s})

	t := s.getT("test")
	println(t != nil)
}

// Output:
// true
