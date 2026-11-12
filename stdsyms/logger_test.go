package stdsyms

import (
	"io"
	"log"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestPluginLoggerSame checks that concurrent calls of pluginLogger for one
// native logger all get the same plugin logger (log.Default() ==
// log.Default() in a plugin), that PluginLogger finds it, and that
// NativeLogger gives the native logger back.
func TestPluginLoggerSame(t *testing.T) {
	l := log.New(io.Discard, "", 0)
	const n = 64
	got := make([]*logLogger, n)
	var wg sync.WaitGroup
	for i := range got {
		wg.Go(func() { got[i] = pluginLogger(l) })
	}
	wg.Wait()
	for i, w := range got {
		if w != got[0] {
			t.Fatalf("plugin logger %d is %p, want %p", i, w, got[0])
		}
	}
	w := got[0]
	if w.l != l {
		t.Fatalf("plugin logger wraps %p, want %p", w.l, l)
	}
	rv, ok := PluginLogger(l)
	if !ok || rv.Interface() != w {
		t.Fatalf("PluginLogger = %v, %v; want %p", rv, ok, w)
	}
	nv, ok := NativeLogger(reflect.ValueOf(w))
	if !ok || nv.Interface() != l {
		t.Fatalf("NativeLogger = %v, %v; want %p", nv, ok, l)
	}
	nv, ok = NativeLogger(reflect.ValueOf((*logLogger)(nil)))
	if !ok || !nv.IsNil() || nv.Type() != LoggerType {
		t.Fatalf("NativeLogger(nil) = %v, %v; want a nil *log.Logger", nv, ok)
	}
	if _, ok := NativeLogger(reflect.ValueOf(l)); ok {
		t.Fatal("NativeLogger of a native logger reports a plugin logger")
	}
	if _, ok := PluginLogger(nil); ok {
		t.Fatal("PluginLogger(nil) reports a plugin logger")
	}
	runtime.KeepAlive(got)
}

// TestPluginLoggerDead checks that the map does not keep a plugin logger
// alive: once the plugin drops it, PluginLogger finds nothing (the native
// logger alone stays), and the next pluginLogger makes a new one.
func TestPluginLoggerDead(t *testing.T) {
	l := log.New(io.Discard, "", 0)
	first := pluginLogger(l)
	if _, ok := PluginLogger(l); !ok {
		t.Fatal("PluginLogger finds nothing while the plugin logger is alive")
	}
	runtime.KeepAlive(first)
	first = nil
	gone := func() bool {
		_, ok := PluginLogger(l)
		return !ok
	}
	for deadline := time.Now().Add(10 * time.Second); !gone(); {
		if time.Now().After(deadline) {
			t.Fatal("the map keeps the dropped plugin logger alive")
		}
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	w := pluginLogger(l)
	if w == nil || w.l != l {
		t.Fatalf("new plugin logger %v does not wrap %p", w, l)
	}
	if rv, ok := PluginLogger(l); !ok || rv.Interface() != w {
		t.Fatalf("PluginLogger = %v, %v after a new plugin logger; want %p", rv, ok, w)
	}
	runtime.KeepAlive(w)
}
