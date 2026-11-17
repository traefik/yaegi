package host

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestLeaks checks the leak check: after the context of a
// configuration is canceled (the next reload), Leaks.Check reports the
// instance whose New left goroutines, once, and not the instance whose
// goroutine ended with the context, nor an instance of the live
// configuration. It forgets the instances that it reported or that ended.
func TestLeaks(t *testing.T) {
	o := Options()
	o.Dir = ".."
	p, err := Load(o, "github.com/traefik/yaegi/v2/internal/pluginhost/testdata/leakplug")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	newIn := func(ctx context.Context, leak int, withCtx bool, l *Leaks, name string) {
		t.Helper()
		res, err := p.Call(context.Background(), "CreateConfig")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]any{"leak": leak, "withCtx": withCtx})
		if err := res[0].Decode(func(ptr any) error { return json.Unmarshal(data, ptr) }); err != nil {
			t.Fatal(err)
		}
		inst := p.NewInstance()
		if _, err := p.Call(inst.Context(ctx), "New", ctx, http.Handler(http.NotFoundHandler()), res[0], name); err != nil {
			t.Fatal(err)
		}
		l.Add(ctx, inst, name)
	}
	var l Leaks
	old, cancelOld := context.WithCancel(context.Background())
	newIn(old, 2, false, &l, "leaky")
	newIn(old, 0, true, &l, "clean")
	live, cancelLive := context.WithCancel(context.Background())
	defer cancelLive()
	newIn(live, 2, false, &l, "live")
	if got := l.Check(0, 0); len(got) != 0 {
		t.Fatalf("before the cancel: %+v, want none", got)
	}
	cancelOld()
	// context.AfterFunc marks the cancel on a goroutine of its own: wait
	// for it, so that the check below runs during the delay and not
	// before the cancel is seen (then it proved nothing about the delay).
	deadline := time.Now().Add(5 * time.Second)
	for {
		l.mu.Lock()
		marked := 0
		for _, e := range l.entries {
			if e.done.Load() != 0 {
				marked++
			}
		}
		l.mu.Unlock()
		if marked == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d instances marked as canceled after 5s, want 2", marked)
		}
		time.Sleep(time.Millisecond)
	}
	if got := l.Check(0, time.Hour); len(got) != 0 {
		t.Fatalf("during the delay: %+v, want none", got)
	}
	time.Sleep(50 * time.Millisecond) // the goroutine of "clean" ends with its context
	got := l.Check(0, 10*time.Millisecond)
	if len(got) != 1 || got[0] != (Leak{Name: "leaky", Goroutines: 2}) {
		t.Fatalf("after the cancel: %+v, want [{leaky 2}]", got)
	}
	if got := l.Check(0, 0); len(got) != 0 {
		t.Fatalf("second check: %+v, want none (each instance once)", got)
	}
	if n := l.Len(); n != 1 {
		t.Fatalf("Leaks follows %d instances, want 1 (the live one)", n)
	}
}
