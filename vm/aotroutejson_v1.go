//go:build !goexperiment.jsonv2

package vm

// json.Encoder and json.Decoder in default mode (see aotroute.go).
//
// json.Marshal and json.Unmarshal go to the AOT copy of json when their
// value holds an interpreted type: the copy sees the interpreted types and
// their methods. The Encoder and the Decoder are native objects of the
// program, with a state, and their methods are not package functions:
//   - Encoder.Encode of such a value runs on a new AOT Encoder made with the
//     state of the native one (writer, indent, escapeHTML), and gives back
//     its write error, which stays in the native Encoder as in Go;
//   - the first Decoder.Decode of such a value makes an AOT Decoder, the
//     twin, with the state of the native one (reader, buffered data,
//     offsets, error, token state, options). From then on, every method of
//     the native Decoder runs on its twin, which has the state. The
//     json.Number and json.Delim values of the copy take the native types
//     (routedValType).

import (
	"encoding/json"
	"go/types"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unsafe"
	"weak"

	"golang.org/x/tools/go/ssa"
)

const jsonPath = "encoding/json"

// jsonStreamCall returns the implementation of a call of a method of
// *json.Encoder or *json.Decoder when json is routed to its AOT copy (nil
// otherwise).
func (m *Machine) jsonStreamCall(f *ssa.Function, args []int32) func([]Value) Value {
	if f.Signature.Recv() == nil || f.Pkg == nil || f.Pkg.Pkg.Path() != jsonPath || !m.routed[jsonPath] {
		return nil
	}
	p := aotPkg(jsonPath)
	if p == nil {
		return nil
	}
	name := f.String()
	switch {
	case name == "(*encoding/json.Encoder).Encode":
		newEnc, ok := p.Funcs["NewEncoder"]
		if !ok {
			return nil
		}
		s := m.nativeSite(f, args)
		site := &nativeSite{args: []int32{args[1]}, params: []types.Type{anyT}, results: []types.Type{errorT}, stores: -1}
		return func(regs []Value) Value {
			v := regs[args[1]]
			if !m.routeNeeded(v, false, jsonPath) {
				if err := m.lostCheck(name, v); err != nil {
					return m.lostResult(err, 1) // lostmethods.go
				}
				return m.callNativeSite(s, regs)
			}
			if m.nestedDeep(v, deepSegs*maxBoxDepth, true) {
				return m.tooDeepResult(1)
			}
			enc := m.toNative(regs[args[0]], s.params[0])
			if enc.IsNil() {
				return m.callNativeSite(s, regs) // the nil dereference of Go
			}
			st := enc.Elem()
			if e := fieldOf(st, "err"); !e.IsNil() {
				return m.errValue(e.Interface().(error))
			}
			aot := newEnc.Call([]reflect.Value{fieldOf(st, "w")})[0]
			aot.MethodByName("SetEscapeHTML").Call([]reflect.Value{fieldOf(st, "escapeHTML")})
			if prefix, indent := fieldOf(st, "indentPrefix"), fieldOf(st, "indentValue"); prefix.Len() > 0 || indent.Len() > 0 {
				aot.MethodByName("SetIndent").Call([]reflect.Value{prefix, indent})
			}
			r := m.callAOTRoute(jsonPath, site, aot.MethodByName("Encode"), 0, regs)
			if e := fieldOf(aot.Elem(), "err"); !e.IsNil() {
				fieldOf(st, "err").Set(e) // a write error stays, as in Go
			}
			return r
		}
	case strings.HasPrefix(name, "(*encoding/json.Decoder)."):
		newDec, ok := p.Funcs["NewDecoder"]
		if !ok {
			return nil
		}
		s := m.nativeSite(f, args)
		method := f.Name()
		var site *nativeSite
		if method == "Decode" {
			site = &nativeSite{args: []int32{args[1]}, params: []types.Type{anyT}, results: []types.Type{errorT}, stores: -1}
		}
		return func(regs []Value) Value {
			dec := m.toNative(regs[args[0]], s.params[0])
			if dec.IsNil() {
				return m.callNativeSite(s, regs)
			}
			nd := dec.Interface().(*json.Decoder)
			tw := jsonTwinOf(nd)
			if !tw.IsValid() {
				if method != "Decode" || !m.routeNeeded(regs[args[1]], false, jsonPath) {
					if method == "Decode" {
						if err := m.lostCheck(name, regs[args[1]]); err != nil {
							return m.lostResult(err, 1) // lostmethods.go
						}
					}
					return m.callNativeSite(s, regs)
				}
				tw = jsonTwin(nd, newDec)
			}
			if method == "Decode" {
				return m.callAOTRoute(jsonPath, site, tw.MethodByName("Decode"), 0, regs)
			}
			in := make([]reflect.Value, len(args)-1)
			for i := range in {
				in[i] = m.toNative(regs[args[i+1]], s.params[i+1])
			}
			out := tw.MethodByName(method).Call(in)
			m.nativeRouteErrs(jsonPath, out)
			return m.pack(out, s.results)
		}
	}
	return nil
}

// fieldOf returns the field name of the struct v, settable also when it is
// not exported (the Encoder and Decoder of json and of its AOT copy have the
// same fields: the copy is made from the same source).
func fieldOf(v reflect.Value, name string) reflect.Value {
	f := v.FieldByName(name)
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

// The twins of the native Decoders, by address, checked by a weak pointer:
// a twin does not keep its Decoder alive, and goes away with it.
var jsonTwins sync.Map // uintptr -> *jsonTwinEntry

type jsonTwinEntry struct {
	dec weak.Pointer[json.Decoder]
	aot reflect.Value // *Decoder of the AOT copy
}

// jsonTwinOf returns the twin of the native Decoder d, or an invalid value.
func jsonTwinOf(d *json.Decoder) reflect.Value {
	if e, ok := jsonTwins.Load(uintptr(unsafe.Pointer(d))); ok {
		if e := e.(*jsonTwinEntry); e.dec.Value() == d {
			return e.aot
		}
	}
	return reflect.Value{}
}

// jsonDecoderState are the fields of the state of a Decoder between two
// calls (the scanner starts again at each value), and the options in d.
var jsonDecoderState = []string{"r", "buf", "scanp", "scanned", "err", "tokenState", "tokenStack"}

// jsonTwin makes the twin of the native Decoder d with newDec, the
// NewDecoder of the AOT copy, and the state of d.
func jsonTwin(d *json.Decoder, newDec reflect.Value) reflect.Value {
	src := reflect.ValueOf(d).Elem()
	aot := newDec.Call([]reflect.Value{fieldOf(src, "r")})[0]
	copyDecoderState(aot.Elem(), src)
	k := uintptr(unsafe.Pointer(d))
	e := &jsonTwinEntry{dec: weak.Make(d), aot: aot}
	jsonTwins.Store(k, e)
	runtime.AddCleanup(d, func(t twinKey) { dropJSONTwin(t.k, t.e) }, twinKey{k, e})
	return aot
}

type twinKey struct {
	k uintptr
	e *jsonTwinEntry
}

// dropJSONTwin removes the twin e of a Decoder that is gone, only if it is
// still the entry at k: a new Decoder can get the address of a dead one and
// record its twin before the cleanup of the dead one runs. Deleting by key
// lost the twin of the new Decoder, whose next call then ran on the native
// Decoder with the old buffered data.
func dropJSONTwin(k uintptr, e *jsonTwinEntry) { jsonTwins.CompareAndDelete(k, e) }

func copyDecoderState(dst, src reflect.Value) {
	for _, n := range jsonDecoderState {
		fieldOf(dst, n).Set(fieldOf(src, n))
	}
	dd, sd := fieldOf(dst, "d"), fieldOf(src, "d")
	for _, n := range []string{"useNumber", "disallowUnknownFields"} {
		fieldOf(dd, n).Set(fieldOf(sd, n))
	}
}
