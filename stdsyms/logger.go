package stdsyms

// The plugin loggers (logLogger) and the native *log.Logger they wrap.
//
// In Symbols, log.Logger is logLogger, so a plugin logger is a *logLogger.
// Native code wants a *log.Logger: a plugin that stores its logger in a
// native field (httputil.ReverseProxy.ErrorLog, http.Server.ErrorLog) gives
// the native logger (NativeLogger). When the plugin reads the field back, it
// gets its own logger again (PluginLogger), so the two values are equal as
// in Go.

import (
	"log"
	"reflect"
	"runtime"
	"sync"
	"weak"
)

// loggers maps a native logger to the plugin logger that wraps it. Both
// pointers are weak: an entry does not keep a logger alive, and goes away
// with its plugin logger.
var loggers sync.Map // weak.Pointer[log.Logger] -> weak.Pointer[logLogger]

// pluginLogger returns the plugin logger of l: the one that already wraps
// it, or a new one.
func pluginLogger(l *log.Logger) *logLogger {
	k := weak.Make(l)
	for {
		old, found := loggers.Load(k)
		if found {
			if w := old.(weak.Pointer[logLogger]).Value(); w != nil {
				return w
			}
		}
		w := &logLogger{l}
		wp := weak.Make(w)
		// Replace only the dead entry seen above: another goroutine may
		// have put a live plugin logger in its place, which must stay the
		// only one (so that two concurrent log.Default() are equal).
		if found {
			if !loggers.CompareAndSwap(k, old, wp) {
				continue
			}
		} else if _, loaded := loggers.LoadOrStore(k, wp); loaded {
			continue
		}
		// Delete the entry only if it is still this one: a new plugin
		// logger may have taken the place of a dead one.
		runtime.AddCleanup(w, func(wp weak.Pointer[logLogger]) { loggers.CompareAndDelete(k, wp) }, wp)
		return w
	}
}

// LoggerType is the native type of the loggers in native code.
var LoggerType = reflect.TypeFor[*log.Logger]()

// NativeLogger returns the native *log.Logger of rv when rv is a plugin
// logger (a *log.Logger for the plugin, see Symbols).
func NativeLogger(rv reflect.Value) (reflect.Value, bool) {
	if !rv.IsValid() || rv.Type() != reflect.TypeFor[*logLogger]() {
		return rv, false
	}
	w := rv.Interface().(*logLogger)
	if w == nil {
		return reflect.Zero(LoggerType), true
	}
	return reflect.ValueOf(w.l), true
}

// PluginLogger returns the plugin logger that wraps the native logger l, if
// a plugin made one (log.New, log.Default).
func PluginLogger(l *log.Logger) (reflect.Value, bool) {
	if l == nil {
		return reflect.Value{}, false
	}
	v, ok := loggers.Load(weak.Make(l))
	if !ok {
		return reflect.Value{}, false
	}
	w := v.(weak.Pointer[logLogger]).Value()
	if w == nil {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(w), true
}
