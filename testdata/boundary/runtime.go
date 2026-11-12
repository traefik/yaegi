// Native boundary of runtime/trace and runtime/pprof: program types given
// to them (docs/BOUNDARY.md). Only deterministic facts are printed.
package main

import (
	"context"
	"fmt"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"sync"
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

// sink is a program writer that counts the bytes written to it.
type sink struct {
	mu sync.Mutex
	n  int
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.n += len(p)
	s.mu.Unlock()
	return len(p), nil
}

func (s *sink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// lines is a writer of an uncomparable type.
type lines []string

func (l lines) Write(p []byte) (int, error) { return len(p), nil }

// res is a resource of a profile.
type res struct{ name string }

// slice is a resource of an uncomparable type.
type slice []int

func main() {
	// row: runtime/trace (*FlightRecorder).Start panic == of two interfaces: comparing uncomparable type
	probe("FlightRecorder", func() {
		fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{})
		out("Start: %v", fr.Start())
		out("Enabled: %v", fr.Enabled())
		s := &sink{}
		n, err := fr.WriteTo(s)
		out("WriteTo: %v %v %v", n > 0, err, s.count() > 0)
		_, err = fr.WriteTo(lines{})
		out("WriteTo uncomparable: %v", err)
		fr.Stop()
		out("Enabled after Stop: %v", fr.Enabled())
	})
	// row: runtime/pprof (*Profile).Add panic map key of interface type: hash of unhashable type
	probe("Profile.Add", func() {
		p := pprof.NewProfile("boundary.program")
		defer func() {
			if r := recover(); r != nil {
				out("panic %T: %v", r, r)
			}
			out("Count: %d", p.Count())
		}()
		r1, r2 := &res{"a"}, res{"b"}
		p.Add(r1, 0)
		p.Add(r2, 0)
		p.Add(42, 0)
		out("Count: %d", p.Count())
		p.Remove(r1)
		p.Remove(r2)
		out("Count after Remove: %d", p.Count())
		var sb strings.Builder
		out("WriteTo: %v %v", p.WriteTo(&sb, 1), strings.HasPrefix(sb.String(), "boundary.program profile: total 1"))
		func() {
			defer func() {
				r := recover()
				out("duplicate: panic %T: %v", r, r)
			}()
			p.Add(42, 0)
		}()
		p.Add(slice{1}, 0)
	})
	// last: under yaegi v2, this probe kills the program (a goroutine of
	// runtime/trace panics), so it does not hide the other probes.
	// row: runtime/trace Start panic == of two interfaces: comparing uncomparable type
	probe("trace.Start", func() {
		s := &sink{}
		out("Start: %v", trace.Start(s))
		out("IsEnabled: %v", trace.IsEnabled())
		out("second Start fails: %v", trace.Start(&sink{}) != nil)
		trace.Log(context.Background(), "cat", "msg")
		trace.Stop()
		out("written: %v", s.count() > 0)
		// trace.Start(lines{}) would panic in a goroutine of runtime/trace
		// (comparing uncomparable type main.lines) and kill the program.
	})
}
