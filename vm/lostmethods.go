package vm

// Methods that native encoding/json and encoding/xml cannot call, in
// default mode.
//
// Native code cannot see the methods of interpreted types (the reflect
// wall, aotroute.go). Native json calls the UnmarshalJSON, UnmarshalText
// and MarshalJSON methods of the value itself through wrappers
// (decodedst.go, vmObj.MarshalJSON), but not the methods of the types
// inside it: a struct field, a slice element, a map key. For these, the
// call goes to the AOT copy of the package (aotroute.go,
// aotroutejson.go), which sees them. Some calls cannot go there:
//
//   - the AOT copy of the package is not compiled into the host (the host
//     does not import aot/all), or the host forced the native mode of the
//     package (Options.Modes);
//   - the value holds a named type of encoding/xml (xml.Name in a struct):
//     the AOT copy only knows its own copies of these types (holdsPkgType);
//   - the stream methods of encoding/xml (Decoder.Decode, Encoder.Encode)
//     have no route.
//
// Native code then ignores the methods: the result is wrong, with no error
// (a golang-jwt token with "exp" was refused). Here such a call is never
// silent: when the type of the value is known at compile time, the call
// is a problem of the load-time check (preflight.go), which fails the load;
// otherwise (a value given in an `any`), the call returns an error that
// names the method and asks for AOT mode.

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// lostSite is a native function or method that calls methods of the types
// inside its `any` argument.
type lostSite struct {
	path   string // the package, a key of lostNames
	arg    int    // index of the `any` argument in the ssa arguments (the receiver first)
	decode bool   // it fills the value (else it reads it)
	route  bool   // aotroute.go or aotroutejson.go can send it to the AOT copy
	name   string // the ssa name of the function (set by init)
}

// lostSites are the functions that lose methods, by ssa name.
var lostSites = map[string]lostSite{
	"encoding/json.Marshal":                 {"encoding/json", 0, false, true, ""},
	"encoding/json.MarshalIndent":           {"encoding/json", 0, false, true, ""},
	"encoding/json.Unmarshal":               {"encoding/json", 1, true, true, ""},
	"(*encoding/json.Encoder).Encode":       {"encoding/json", 1, false, true, ""},
	"(*encoding/json.Decoder).Decode":       {"encoding/json", 1, true, true, ""},
	"encoding/xml.Marshal":                  {"encoding/xml", 0, false, true, ""},
	"encoding/xml.MarshalIndent":            {"encoding/xml", 0, false, true, ""},
	"encoding/xml.Unmarshal":                {"encoding/xml", 1, true, true, ""},
	"(*encoding/xml.Encoder).Encode":        {"encoding/xml", 1, false, false, ""},
	"(*encoding/xml.Encoder).EncodeElement": {"encoding/xml", 1, false, false, ""},
	"(*encoding/xml.Decoder).Decode":        {"encoding/xml", 1, true, false, ""},
	"(*encoding/xml.Decoder).DecodeElement": {"encoding/xml", 1, true, false, ""},
}

func init() {
	for name, s := range lostSites {
		s.name = name
		lostSites[name] = s
	}
}

// topHandled reports whether native code with the wrappers of the VM calls
// the method name of the value itself (not of a type inside it).
func topHandled(path string, decode bool, name string) bool {
	if path != "encoding/json" {
		return false
	}
	if decode {
		return name == "UnmarshalJSON" || name == "UnmarshalText" // decodedst.go
	}
	return name == "MarshalJSON" // vmObj.MarshalJSON
}

// lostSiteOf returns the lostSite of the native function f, if it has one.
func lostSiteOf(f *ssa.Function) (lostSite, bool) {
	if f.Pkg == nil {
		return lostSite{}, false
	}
	s, ok := lostSites[f.String()]
	return s, ok
}

// routable reports whether a call of site s with a value of type t goes to
// the AOT copy (see aotRouteCall and jsonStreamCall). why tells why not.
func (m *Machine) routable(s lostSite, t types.Type) (ok bool, why string) {
	if m.modeOf(s.path) != ModeNative {
		return true, "" // the call runs the AOT copy, or the package from source
	}
	switch {
	case !s.route:
		return false, "the stream methods of " + s.path + " have no route to its AOT copy"
	case m.vr == nil || !m.routed[s.path]:
		if _, explicit := m.Modes[s.path]; explicit {
			return false, "the host sets the mode of " + s.path + " to native"
		}
		return false, "the AOT copy of " + s.path + " is not compiled into the host"
	case mentionsPkg(t, s.path, map[types.Type]bool{}):
		return false, "the value also holds a type of " + s.path + " that the AOT copy cannot take"
	}
	return true, ""
}

// lostMethod returns the first method of an interpreted type inside t that
// the native function of site s would not call ("main.T.UnmarshalJSON"),
// or "".
func (m *Machine) lostMethod(s lostSite, t types.Type) string {
	w := lostWalk{m: m, s: s, seen: map[lostKey]bool{}}
	return w.walk(t, true, s.decode, lostElem)
}

type lostWalk struct {
	m    *Machine
	s    lostSite
	seen map[lostKey]bool
}

// lostPlace is where a value is in the document: it selects the methods
// that json and xml call for it.
type lostPlace int

const (
	lostElem   lostPlace = iota // a json value, an xml element
	lostAttr                    // an xml attribute (`xml:",attr"`)
	lostChar                    // xml character data (`xml:",chardata"`)
	lostMapKey                  // a json map key
)

// names returns the methods that the package of the site calls for a
// value at place p (encoding/json encode.go and decode.go, encoding/xml
// marshal.go and read.go).
func (w *lostWalk) names(p lostPlace) []string {
	decode := w.s.decode
	if w.s.path == "encoding/json" {
		switch {
		case p == lostMapKey && decode:
			// a key is read with UnmarshalText, written with MarshalText
			// (unless it is a string, see walk): never with UnmarshalJSON
			// or MarshalJSON
			return []string{"UnmarshalText"}
		case p == lostMapKey:
			return []string{"MarshalText"}
		case decode:
			return []string{"UnmarshalJSON", "UnmarshalText"}
		}
		return []string{"MarshalJSON", "MarshalText"}
	}
	switch {
	case p == lostAttr && decode:
		return []string{"UnmarshalXMLAttr", "UnmarshalText"}
	case p == lostAttr:
		return []string{"MarshalXMLAttr", "MarshalText"}
	case p == lostChar && decode:
		return []string{"UnmarshalText"}
	case p == lostChar:
		return []string{"MarshalText"}
	case decode:
		return []string{"UnmarshalXML", "UnmarshalText"}
	}
	return []string{"MarshalXML", "MarshalText"}
}

type lostKey struct {
	t         types.Type
	top, addr bool
	p         lostPlace
}

// walk looks for a lost method in t, a value at place p. top is set for the
// value itself (and what it points to); addr is set when the value is
// addressable, so that the methods of *t count.
func (w *lostWalk) walk(t types.Type, top, addr bool, p lostPlace) string {
	t = types.Unalias(t)
	k := lostKey{t, top, addr, p}
	if w.seen[k] {
		return ""
	}
	w.seen[k] = true
	switch u := t.(type) {
	case *types.Named:
		if w.m.isNativeNamed(u) {
			return "" // native: native code sees its methods
		}
		if _, ok := u.Underlying().(*types.Interface); ok {
			return "" // the dynamic value is checked when it is known
		}
		mt := types.Type(u)
		if addr {
			mt = types.NewPointer(u)
		}
		ms := types.NewMethodSet(mt) // not prog.MethodSets: this runs on any goroutine
		for _, name := range w.names(p) {
			if ms.Lookup(nil, name) == nil {
				continue
			}
			if top && topHandled(w.s.path, w.s.decode, name) {
				// native code calls the method of the value itself
				// (also one promoted from an embedded field) and never
				// looks inside the value
				return ""
			}
			return typeString(u) + "." + name
		}
		return w.walk(u.Underlying(), false, addr, p)
	case *types.Pointer:
		return w.walk(u.Elem(), top, true, p)
	case *types.Struct:
		return w.walkFields(u, addr)
	case *types.Slice:
		return w.walk(u.Elem(), false, true, p)
	case *types.Array:
		return w.walk(u.Elem(), false, addr, p)
	case *types.Map:
		if w.s.path != "encoding/json" {
			return "" // xml refuses a map: no method is called
		}
		// a key of string kind is written as that string
		if kt, ok := u.Key().Underlying().(*types.Basic); !ok || w.s.decode || kt.Kind() != types.String {
			if r := w.walk(u.Key(), false, w.s.decode, lostMapKey); r != "" {
				return r
			}
		}
		return w.walk(u.Elem(), false, w.s.decode, lostElem)
	}
	return ""
}

// walkFields looks for a lost method in the fields of the struct u, as
// json and xml see them: an unexported field is skipped (also an embedded
// one of a non-struct type), as a `-` field; an embedded struct is
// flattened, so that its own methods do not count (the ones promoted to
// the outer type were counted there): json flattens it only without a
// name in its tag, xml always; the tag of an xml field tells whether it is
// an attribute or character data.
func (w *lostWalk) walkFields(u *types.Struct, addr bool) string {
	xml := w.s.path == "encoding/xml"
	key := "json"
	if xml {
		key = "xml"
	}
	for i := 0; i < u.NumFields(); i++ {
		f := u.Field(i)
		ft := f.Type()
		var st *types.Struct // the struct of an embedded struct field
		faddr := addr
		if f.Embedded() {
			et := ft
			if pt, ok := et.Underlying().(*types.Pointer); ok {
				et = pt.Elem()
				faddr = true
			}
			st, _ = et.Underlying().(*types.Struct)
			if st == nil && !f.Exported() {
				continue
			}
		} else if !f.Exported() {
			continue
		}
		tag := reflect.StructTag(u.Tag(i)).Get(key)
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if st != nil && (xml || name == "") {
			// through walk: seen stops a type embedded in itself
			if r := w.walk(st, false, faddr, lostElem); r != "" {
				return r
			}
			continue
		}
		p := lostElem
		if xml {
			switch {
			case hasOpt(opts, "attr"):
				p = lostAttr
			case hasOpt(opts, "chardata"):
				p = lostChar
			case hasOpt(opts, "comment"), hasOpt(opts, "innerxml"):
				continue // a string or []byte: no method
			}
		}
		if r := w.walk(ft, false, addr, p); r != "" {
			return r
		}
	}
	return ""
}

// hasOpt reports whether the options of a struct tag ("a,b") hold opt.
func hasOpt(opts, opt string) bool {
	for opts != "" {
		var o string
		o, opts, _ = strings.Cut(opts, ",")
		if o == opt {
			return true
		}
	}
	return false
}

// lostProblem returns the message of the load-time problem of a call of
// the native function f with the ssa arguments args, or "": the type of
// the value is known (the `any` argument is made from a concrete type),
// the call cannot go to the AOT copy, and native code would not call a
// method of a type inside the value.
func (m *Machine) lostProblem(f *ssa.Function, args []ssa.Value) string {
	s, ok := lostSiteOf(f)
	if !ok || s.arg >= len(args) || m.isInterp(f) {
		return ""
	}
	mi, ok := args[s.arg].(*ssa.MakeInterface)
	if !ok {
		return ""
	}
	t := mi.X.Type()
	routeOK, why := m.routable(s, t)
	if routeOK {
		return ""
	}
	lost := m.lostMethod(s, t)
	if lost == "" {
		return ""
	}
	return "unsupported: " + lostMsg(s.name, lost, typeString(t), why)
}

// lostMsg is the message for a call of the function name (an ssa name)
// that would not call the method lost inside a value of type t.
func lostMsg(name, lost, t, why string) string {
	if strings.HasPrefix(name, "(*") {
		name = strings.Replace(strings.TrimPrefix(name, "(*"), ").", ".", 1)
	}
	return fmt.Sprintf("in default mode, %s does not call the method %s, found in a value of type %s (%s); run the plugin in AOT mode (Options.AOT)", name, lost, t, why)
}

// LostMethodError is the error of a call of encoding/json or encoding/xml
// that would ignore a method of an interpreted type inside its value, in
// default mode (see lostProblem). The type of the value was not known at
// load time.
type LostMethodError struct{ Msg string }

func (e *LostMethodError) Error() string { return "yaegi v2: " + e.Msg }

type lostRTKey struct {
	rt   *RType
	site string
}

// lostCheck returns the error of a call of the native function of
// lostSites[name] whose `any` argument is v, or nil (cached per dynamic
// type).
func (m *Machine) lostCheck(name string, v Value) error {
	iv, _ := v.R.(*Iface)
	if iv == nil || iv.T.Native != nil || iv.T.T == nil {
		return nil
	}
	s, ok := lostSites[name]
	if !ok {
		return nil
	}
	k := lostRTKey{iv.T, name}
	if r, ok := m.lostTypes.Load(k); ok {
		if r == nil {
			return nil
		}
		return r.(error)
	}
	var err error
	if ok, why := m.routable(s, iv.T.T); !ok {
		if lost := m.lostMethod(s, iv.T.T); lost != "" {
			err = &LostMethodError{Msg: lostMsg(name, lost, typeString(iv.T.T), why)}
		}
	}
	if err == nil {
		m.lostTypes.Store(k, nil)
	} else {
		m.lostTypes.Store(k, err)
	}
	return err
}

// lostResult is the result of a call that returns the error err: the zero
// values of the other results, and err last.
func (m *Machine) lostResult(err error, nres int) Value {
	e := m.ErrorVal(err)
	if nres == 1 {
		return e
	}
	vs := make([]Value, nres)
	vs[nres-1] = e
	return Value{R: vs}
}

// lostCall returns the implementation of a call of the native function f
// of a lostSite that does not go to the AOT copy: the native call, after a
// check of the dynamic type of the value (nil when f has no lostSite).
func (m *Machine) lostCall(f *ssa.Function, args []int32) func([]Value) Value {
	s, ok := lostSiteOf(f)
	if !ok || s.arg >= len(args) {
		return nil
	}
	site := m.nativeSite(f, args)
	ar := args[s.arg]
	nres := f.Signature.Results().Len()
	return func(regs []Value) Value {
		if err := m.lostCheck(s.name, regs[ar]); err != nil {
			return m.lostResult(err, nres)
		}
		return m.callNativeSite(site, regs)
	}
}
