package host

// The calls of a Traefik plugin. A plugin follows one shape: a CreateConfig
// function that returns its configuration (a pointer to a struct), and a New
// function that returns an http.Handler (a middleware) or a value with Init,
// Provide and Stop methods (a provider). The vm package knows nothing of
// this shape: it calls functions and methods by name. These helpers are the
// part of a Traefik adapter that does not depend on Traefik.

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"net/http"

	"github.com/traefik/yaegi/v2/vm"
)

// Kind is the kind of a plugin: what its New function returns.
type Kind int

const (
	// Middleware: New(ctx context.Context, next http.Handler, cfg *Config,
	// name string) (http.Handler, error).
	Middleware Kind = iota + 1
	// Provider: New(ctx context.Context, cfg *Config, name string) (P,
	// error), where P has the methods Init() error, Provide(chan<-
	// json.Marshaler) error and Stop() error.
	Provider
)

func (k Kind) String() string {
	switch k {
	case Middleware:
		return "middleware"
	case Provider:
		return "provider"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// EntryError is the error of CheckEntryPoints: the plugin does not have the
// functions or methods that the host calls. It is a *vm.LoadError for
// errors.As too (Unwrap): the plugin is broken, as for a type error.
type EntryError struct {
	Kind Kind
	Msg  string
}

func (e *EntryError) Error() string { return fmt.Sprintf("host: %s plugin: %s", e.Kind, e.Msg) }

// Unwrap returns a *vm.LoadError with the same message, so that a host that
// tells a broken plugin by errors.As(err, **vm.LoadError) sees this one too.
func (e *EntryError) Unwrap() error { return &vm.LoadError{Msg: e.Error()} }

// LoadKind is LoadContext, then CheckEntryPoints for kind: a plugin without
// the entry points of its kind fails at load, as under yaegi (Traefik reads
// New and CreateConfig when it builds the plugin, not at each router build).
func LoadKind(ctx context.Context, opts vm.Options, pkgPath string, kind Kind) (*vm.Plugin, error) {
	p, err := LoadContext(ctx, opts, pkgPath)
	if err != nil {
		return nil, err
	}
	if err := CheckEntryPoints(p, kind); err != nil {
		return nil, err
	}
	return p, nil
}

// CheckEntryPoints returns an *EntryError when p does not have the entry
// points of kind, with the types the host calls them with:
//   - CreateConfig: an exported function without parameters and with one
//     result;
//   - New of a Middleware: (context.Context, http.Handler, C, string) and
//     the results (H, error), where C takes the result of CreateConfig and
//     H is an http.Handler or an interface type (any: NewInstanceHandler
//     checks the value, as Traefik with yaegi does);
//   - New of a Provider: (context.Context, C, string) and the results (P,
//     error), where P has the methods Init() error, Provide(T) error with
//     one parameter, and Stop() error.
//
// A parameter may have any type that the value of the host is assignable
// to: an interface with the ServeHTTP method for http.Handler, any for the
// config. It only reads the types of the plugin: no plugin code runs.
func CheckEntryPoints(p *vm.Plugin, kind Kind) error {
	bad := func(format string, a ...any) error {
		return &EntryError{Kind: kind, Msg: fmt.Sprintf(format, a...)}
	}
	if kind != Middleware && kind != Provider {
		return bad("unknown kind")
	}
	path := p.Pkg.Pkg.Path()
	sigOf := func(name string) (*types.Signature, error) {
		f, ok := p.Pkg.Pkg.Scope().Lookup(name).(*types.Func)
		if !ok {
			return nil, bad("package %s has no function %s", path, name)
		}
		return f.Type().(*types.Signature), nil
	}
	cc, err := sigOf("CreateConfig")
	if err != nil {
		return err
	}
	if cc.Params().Len() != 0 || cc.Results().Len() != 1 || cc.Variadic() {
		return bad("CreateConfig is %s, want a function without parameters and with one result", cc)
	}
	cfg := cc.Results().At(0).Type()
	nw, err := sigOf("New")
	if err != nil {
		return err
	}
	want := "func(ctx context.Context, next http.Handler, config C, name string) (http.Handler, error)"
	nparams := 4
	if kind == Provider {
		want = "func(ctx context.Context, config C, name string) (P, error)"
		nparams = 3
	}
	ps, rs := nw.Params(), nw.Results()
	if ps.Len() != nparams || rs.Len() != 2 || nw.Variadic() {
		return bad("New is %s, want %s", nw, want)
	}
	if !takesHostType(p.Pkg.Pkg, ps.At(0).Type(), "context", "Context") || !isString(ps.At(nparams-1).Type()) || !isError(rs.At(1).Type()) {
		return bad("New is %s, want %s", nw, want)
	}
	if c := ps.At(nparams - 2).Type(); !types.AssignableTo(cfg, c) {
		return bad("New takes a %s, but CreateConfig returns a %s", c, cfg)
	}
	if kind == Middleware {
		if !takesHostType(p.Pkg.Pkg, ps.At(1).Type(), "net/http", "Handler") {
			return bad("New is %s, want %s", nw, want)
		}
		// An interface result (any) is checked at New, as yaegi does it
		// (NewInstanceHandler).
		res := rs.At(0).Type()
		if types.IsInterface(res) {
			return nil
		}
		h := hostType(p.Pkg.Pkg, "net/http", "Handler")
		if h == nil || !types.Implements(res, h.Underlying().(*types.Interface)) {
			return bad("New returns a %s, which is not an http.Handler", res)
		}
		return nil
	}
	res := rs.At(0).Type()
	ms := types.NewMethodSet(res)
	for _, m := range []struct {
		name    string
		nparams int
	}{{"Init", 0}, {"Provide", 1}, {"Stop", 0}} {
		sel := ms.Lookup(nil, m.name)
		if sel == nil {
			return bad("New returns a %s, which has no method %s", res, m.name)
		}
		sig := sel.Type().(*types.Signature)
		if sig.Params().Len() != m.nparams || sig.Results().Len() != 1 || !isError(sig.Results().At(0).Type()) {
			return bad("the method %s of %s is %s, want %d parameters and an error result", m.name, res, sig, m.nparams)
		}
	}
	return nil
}

// takesHostType reports whether a parameter of type t takes the value of
// type pkg.name that the host gives: the type is assignable to t (the type
// itself, an interface with fewer methods, any). When the plugin does not
// import pkg, even indirectly, only an interface without methods takes it.
func takesHostType(root *types.Package, t types.Type, pkg, name string) bool {
	if ht := hostType(root, pkg, name); ht != nil {
		return types.AssignableTo(ht, t)
	}
	i, ok := t.Underlying().(*types.Interface)
	return ok && i.Empty()
}

// hostType returns the type pkg.name as the plugin root sees it, from the
// packages root imports directly or indirectly, or nil when it imports no
// package pkg.
func hostType(root *types.Package, pkg, name string) types.Type {
	seen := map[*types.Package]bool{}
	todo := []*types.Package{root}
	for len(todo) > 0 {
		p := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if seen[p] {
			continue
		}
		seen[p] = true
		if p.Path() == pkg {
			if tn, ok := p.Scope().Lookup(name).(*types.TypeName); ok {
				return tn.Type()
			}
			return nil
		}
		todo = append(todo, p.Imports()...)
	}
	return nil
}

func isString(t types.Type) bool {
	b, ok := types.Unalias(t).(*types.Basic)
	return ok && b.Kind() == types.String
}

func isError(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

// CreateConfig calls the CreateConfig function of p and returns its result,
// which the host fills from the configuration (vm.Object.Decode) and gives
// to New.
func CreateConfig(p *vm.Plugin) (*vm.Object, error) {
	res, err := p.Call(context.Background(), "CreateConfig")
	if err != nil {
		return nil, err
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("the CreateConfig function returns %d results, 1 expected", len(res))
	}
	return res[0], nil
}

// NewHandler calls New(ctx, next, cfg, name) (http.Handler, error) of p,
// in a new vm.Instance (NewInstanceHandler).
func NewHandler(ctx context.Context, p *vm.Plugin, next http.Handler, cfg *vm.Object, name string) (http.Handler, error) {
	h, _, err := NewInstanceHandler(ctx, p, next, cfg, name)
	return h, err
}

// NewInstanceHandler calls New(ctx, next, cfg, name) (http.Handler, error)
// of p in a new vm.Instance, and returns the instance too. This is the
// reload model of docs/RELOAD.md: one Plugin for the life of the host, and
// New at each reload. The goroutines that New starts belong to the
// instance: MaxGoroutines bounds them per New, so the goroutines that the
// New calls of old reloads left (a cache janitor that never ends) do not
// make a new New fail. inst.Goroutines shows them (Leaks reports the
// ones that outlive their configuration). inst.Close stops them, for hosts
// that know when an instance is gone; the Traefik adapter does not call it.
// The plugin gets ctx itself in New, not the context of the instance.
func NewInstanceHandler(ctx context.Context, p *vm.Plugin, next http.Handler, cfg *vm.Object, name string) (h http.Handler, inst *vm.Instance, err error) {
	inst = p.NewInstance()
	res, err := p.Call(inst.Context(ctx), "New", ctx, next, cfg, name)
	if err != nil {
		return nil, inst, err
	}
	if len(res) != 2 {
		return nil, inst, fmt.Errorf("the New function returns %d results, 2 expected", len(res))
	}
	if err := resultErr(res[1]); err != nil {
		return nil, inst, err
	}
	v, err := res[0].Interface()
	if err != nil {
		return nil, inst, err
	}
	h, ok := v.(http.Handler)
	if !ok {
		return nil, inst, fmt.Errorf("the New function returns %T, an http.Handler is expected", v)
	}
	return h, inst, nil
}

// NewProvider calls New(ctx, cfg, name) (T, error) of p, and returns the T
// value, whose methods CallMethod calls. New runs in a new vm.Instance, as
// in NewInstanceHandler.
func NewProvider(ctx context.Context, p *vm.Plugin, cfg *vm.Object, name string) (*vm.Object, error) {
	res, err := p.Call(p.NewInstance().Context(ctx), "New", ctx, cfg, name)
	if err != nil {
		return nil, err
	}
	if len(res) != 2 {
		return nil, fmt.Errorf("the New function returns %d results, 2 expected", len(res))
	}
	if err := resultErr(res[1]); err != nil {
		return nil, err
	}
	return res[0], nil
}

// CallMethod calls the method name of o and returns its results as native
// values (a nil error is a nil any).
func CallMethod(o *vm.Object, name string, args ...any) ([]any, error) {
	res, err := o.Call(context.Background(), name, args...)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(res))
	for i, r := range res {
		if out[i], err = r.Interface(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resultErr returns the error that the error result r holds.
func resultErr(r *vm.Object) error {
	v, err := r.Interface()
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	e, ok := v.(error)
	if !ok {
		return errors.New("the error result is not an error")
	}
	return e
}
