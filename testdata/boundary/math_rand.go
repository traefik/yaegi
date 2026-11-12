// Native boundary of math/rand: program types given to math/rand (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"math/rand"
	"sort"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// lcg is a Source without Uint64 (pointer receivers).
type lcg struct{ n uint64 }

func (s *lcg) Int63() int64 {
	s.n = s.n*6364136223846793005 + 1442695040888963407
	return int64(s.n >> 1)
}
func (s *lcg) Seed(seed int64) { s.n = uint64(seed) }

// lcg64 is a Source64: it has Uint64.
type lcg64 struct {
	*lcg
	calls *int
}

func (s lcg64) Uint64() uint64 {
	*s.calls++
	s.lcg.n = s.lcg.n*2862933555777941757 + 3037000493
	return s.lcg.n
}
func (s lcg64) Int63() int64    { return int64(s.Uint64() >> 1) }
func (s lcg64) Seed(seed int64) { s.lcg.n = uint64(seed) }

// gsrc is a generic Source with Uint64.
type gsrc[T ~uint64] struct{ n *T }

func (s gsrc[T]) Uint64() uint64 {
	*s.n = *s.n*6364136223846793005 + 1
	return uint64(*s.n)
}
func (s gsrc[T]) Int63() int64    { return int64(s.Uint64() >> 1) }
func (s gsrc[T]) Seed(seed int64) { *s.n = T(seed) }

type myU uint64

// use prints the results of every method of r.
func use(name string, r *rand.Rand) {
	out("%s: Int63 %d Uint32 %d Uint64 %d Int31 %d Int %d", name, r.Int63(), r.Uint32(), r.Uint64(), r.Int31(), r.Int())
	out("%s: Int63n %d Int31n %d Intn %d", name, r.Int63n(1000), r.Int31n(1000), r.Intn(1000))
	out("%s: Float64 %.6f Float32 %.6f Norm %.6f Exp %.6f", name, r.Float64(), r.Float32(), r.NormFloat64(), r.ExpFloat64())
	out("%s: Perm %v", name, r.Perm(5))
	s := []int{1, 2, 3, 4, 5}
	r.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
	out("%s: Shuffle %v", name, s)
	b := make([]byte, 6)
	n, err := r.Read(b)
	out("%s: Read %d %v %x", name, n, err, b)
	r.Seed(42)
	out("%s: after Seed %d", name, r.Int63())
}

func main() {
	// row: math/rand New assert x.(math/rand.Source64)
	probe("New", func() {
		use("lcg", rand.New(&lcg{1}))
		calls := 0
		use("lcg64", rand.New(lcg64{&lcg{1}, &calls}))
		out("lcg64 Uint64 calls > 0: %v", calls > 0)
		var n myU = 1
		use("gsrc", rand.New(gsrc[myU]{&n}))
		var src rand.Source = &lcg{7}
		use("iface", rand.New(src))
		use("NewSource", rand.New(rand.NewSource(1)))
	})
	// row: math/rand ExpFloat64,Float32,Float64,Int,Int31,Int31n,Int63,Int63n,Intn,NormFloat64,Perm,Read,Seed,Shuffle,Uint32,Uint64 assert x.(math/rand.Source64)
	probe("global", func() {
		// the global source is not a program type: only print facts
		out("Float64 in range: %v", rand.Float64() < 1)
		out("Float32 in range: %v", rand.Float32() < 1)
		out("ExpFloat64 positive: %v", rand.ExpFloat64() >= 0)
		n := rand.NormFloat64()
		out("NormFloat64 finite: %v", n == n)
		out("Int positive: %v", rand.Int() >= 0)
		out("Int31 positive: %v", rand.Int31() >= 0)
		out("Int31n: %d", rand.Int31n(1))
		out("Int63 positive: %v", rand.Int63() >= 0)
		out("Int63n: %d", rand.Int63n(1))
		out("Intn: %d", rand.Intn(1))
		_ = rand.Uint32()
		_ = rand.Uint64()
		p := rand.Perm(5)
		sort.Ints(p)
		out("Perm sorted: %v", p)
		s := []int{3, 1, 2}
		rand.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
		sort.Ints(s)
		out("Shuffle sorted: %v", s)
		b := make([]byte, 4)
		k, err := rand.Read(b)
		out("Read: %d %v", k, err)
		rand.Seed(1)
		out("after Seed: %v", rand.Intn(10) < 10)
	})
}
