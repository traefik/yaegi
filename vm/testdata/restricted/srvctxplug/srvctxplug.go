// Package srvctxplug looks for the *http.Server of the host in the context
// of the request (http.ServerContextKey) and replaces its Handler, as a
// field and through its reflect: a restricted plugin must not find it,
// not even in the parents of the context (the exported field Context of a
// context.WithValue). The next handler of the host still gets the server.
//
// A direct ctx.Value(http.ServerContextKey) finds nothing: the key has an
// unexported type of net/http, which the VM passes to native code as a key
// of its own. The reflect of the plugin passes the native key.
package srvctxplug

import (
	"context"
	"net/http"
	"reflect"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func server(ctx context.Context) (*http.Server, bool) {
	if srv, ok := ctx.Value(http.ServerContextKey).(*http.Server); ok {
		return srv, true
	}
	key := any(http.ServerContextKey)
	// Ask each context of the chain, going up through the exported field
	// Context of its struct.
	for c := reflect.ValueOf(ctx); c.IsValid(); {
		v := c.MethodByName("Value").Call([]reflect.Value{reflect.ValueOf(&key).Elem()})[0]
		if v.CanInterface() {
			if srv, ok := v.Interface().(*http.Server); ok {
				return srv, true
			}
		}
		e := c
		for e.Kind() == reflect.Pointer || e.Kind() == reflect.Interface {
			e = e.Elem()
		}
		if e.Kind() != reflect.Struct {
			break
		}
		f := e.FieldByName("Context")
		if !f.IsValid() || !f.CanInterface() || f.IsNil() {
			break
		}
		c = reflect.ValueOf(f.Interface())
	}
	return nil, false
}

// spy is a context of the plugin: it keeps what its parent gives to the
// keys that the vm asks for, when the request goes to the next handler of
// the host.
type spy struct {
	context.Context
	got *[]any
}

func (s spy) Value(key any) any {
	v := s.Context.Value(key)
	*s.got = append(*s.got, v)
	return v
}

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), Config{}, 1)))
			return
		}
		if r.URL.Path == "/spy" {
			var got []any
			next.ServeHTTP(w, r.WithContext(spy{r.Context(), &got}))
			for _, v := range got {
				if srv, ok := v.(*http.Server); ok {
					srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("evil")) })
				}
			}
			return
		}
		if r.URL.Path == "/shape" {
			// A struct of the plugin with the shape of the hidden context:
			// Set and Convert must refuse it, as Go does (the unexported
			// fields of two packages are never the same fields).
			type hidden struct {
				parent context.Context
				srv    any
			}
			var h hidden
			src := reflect.ValueOf(r.Context()).Elem()
			func() {
				defer func() { recover() }()
				reflect.ValueOf(&h).Elem().Set(src)
			}()
			func() {
				defer func() { recover() }()
				h = src.Convert(reflect.TypeOf(hidden{})).Interface().(hidden)
			}()
			if srv, ok := h.srv.(*http.Server); ok {
				srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("evil")) })
			}
			return
		}
		srv, ok := server(r.Context())
		if !ok {
			w.Write([]byte("none"))
			return
		}
		w.Write([]byte("found"))
		if r.URL.Query().Get("write") == "" {
			return
		}
		evil := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("evil")) })
		if r.URL.Path == "/reflect" {
			reflect.ValueOf(srv).Elem().FieldByName("Handler").Set(reflect.ValueOf(evil))
			return
		}
		srv.Handler = evil
	}), nil
}

// Has reports whether ctx holds the server of the host.
func Has(ctx context.Context) bool {
	_, ok := server(ctx)
	return ok
}
