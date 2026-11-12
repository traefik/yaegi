//go:build goexperiment.jsonv2

package vm

// json.Encoder and json.Decoder in default mode (see aotroute.go).
//
// json.Marshal and json.Unmarshal go to the AOT copy of json when their
// value holds an interpreted type: the copy sees the interpreted types and
// their methods. The Encoder and the Decoder are native objects of the
// program, with a state, and their methods are not package functions.
// encoding/json is built on encoding/json/v2 (Go 1.27): the state of a
// stream is in its jsontext.Encoder or jsontext.Decoder, and a value is
// encoded or decoded by json/v2 with the options of the stream (opts):
//   - Encoder.Encode of such a value runs on a new AOT Encoder made with the
//     state of the native one (writer, options, indent), and gives back its
//     write error, which stays in the native Encoder as in Go;
//   - Decoder.Decode of such a value reads the next JSON value with the
//     native Decoder (its jsontext.Decoder keeps the stream state, the other
//     methods stay native), then decodes it with the Unmarshal of the AOT
//     copy of json/v2 and the options of the native Decoder (UseNumber,
//     DisallowUnknownFields), as Decode does with the native json/v2.

import (
	"encoding/json"
	"go/types"
	"reflect"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

const jsonPath = "encoding/json"

// jsonV2Path is the package of the json/v2 Unmarshal that Decoder.Decode
// calls with the options of the Decoder.
const jsonV2Path = "encoding/json/v2"

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
			at := aot.Elem()
			if !copyJSONOptions(fieldOf(at, "opts"), fieldOf(st, "opts")) {
				return m.callNativeSite(s, regs)
			}
			fieldOf(at, "indentPrefix").Set(fieldOf(st, "indentPrefix"))
			fieldOf(at, "indentValue").Set(fieldOf(st, "indentValue"))
			r := m.callAOTRoute(jsonPath, site, aot.MethodByName("Encode"), 0, regs)
			if e := fieldOf(at, "err"); !e.IsNil() {
				fieldOf(st, "err").Set(e) // a write error stays, as in Go
			}
			return r
		}
	case name == "(*encoding/json.Decoder).Decode":
		v2 := aotPkg(jsonV2Path)
		if v2 == nil {
			return nil
		}
		unmarshal, ok1 := v2.Funcs["Unmarshal"]
		defaults, ok2 := p.Funcs["DefaultOptionsV1"]
		if !ok1 || !ok2 {
			return nil
		}
		s := m.nativeSite(f, args)
		site := &nativeSite{args: []int32{args[1]}, params: []types.Type{anyT}, results: []types.Type{errorT}, stores: -1}
		return func(regs []Value) Value {
			if !m.routeNeeded(regs[args[1]], false, jsonPath) {
				if err := m.lostCheck(name, regs[args[1]]); err != nil {
					return m.lostResult(err, 1) // lostmethods.go
				}
				return m.callNativeSite(s, regs)
			}
			dec := m.toNative(regs[args[0]], s.params[0])
			if dec.IsNil() {
				return m.callNativeSite(s, regs) // the nil dereference of Go
			}
			opts := reflect.New(defaults.Type().Out(0)).Elem()
			opts.Set(defaults.Call(nil)[0])
			if !copyJSONOptions(opts, fieldOf(dec.Elem(), "opts")) {
				return m.callNativeSite(s, regs)
			}
			// The native Decode reads the value and keeps the state of the
			// stream (and its error); a RawMessage takes the bytes as they are.
			var raw json.RawMessage
			if err := dec.Interface().(*json.Decoder).Decode(&raw); err != nil {
				return m.errValue(err)
			}
			fn := reflect.ValueOf(func(v any) error {
				out := unmarshal.Call([]reflect.Value{reflect.ValueOf([]byte(raw)), reflect.ValueOf(&v).Elem(), opts})
				err, _ := out[0].Interface().(error)
				return err
			})
			return m.callAOTRoute(jsonPath, site, fn, 0, regs)
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

// copyJSONOptions sets dst, an Options of the AOT copy of json, to a new
// copy of src, the Options of native json: both hold a *jsonopts.Struct
// made from the same source (flags, indent, limits, format). The Options of
// DefaultOptionsV1 point to a variable of the package, which is never
// written: dst gets its own Struct. It reports false when src holds
// something else, or values that the copy cannot take (Marshalers and
// Unmarshalers, which the API of json v1 does not set).
func copyJSONOptions(dst, src reflect.Value) bool {
	if src.Kind() != reflect.Interface || src.IsNil() || src.Elem().Kind() != reflect.Pointer || src.Elem().IsNil() {
		return false
	}
	sv := src.Elem().Elem()
	if dst.IsNil() || dst.Elem().Kind() != reflect.Pointer || sv.Kind() != reflect.Struct {
		return false
	}
	nv := reflect.New(dst.Elem().Type().Elem())
	if !copySameShape(nv.Elem(), sv) {
		return false
	}
	dst.Set(nv)
	return true
}

// copySameShape copies src into dst, two values of types made from the
// same source in two packages (native and AOT copy): structs field by field
// and basic values. Interfaces, pointers and other kinds are only copied
// when they are nil or zero.
func copySameShape(dst, src reflect.Value) bool {
	switch src.Kind() {
	case reflect.Struct:
		if dst.Kind() != reflect.Struct || dst.NumField() != src.NumField() {
			return false
		}
		for i := 0; i < src.NumField(); i++ {
			if dst.Type().Field(i).Name != src.Type().Field(i).Name {
				return false
			}
			df := reflect.NewAt(dst.Field(i).Type(), unsafe.Pointer(dst.Field(i).UnsafeAddr())).Elem()
			if !copySameShape(df, src.Field(i)) {
				return false
			}
		}
		return true
	}
	if dst.Kind() != src.Kind() {
		return src.IsZero() && dst.IsZero()
	}
	switch src.Kind() {
	case reflect.Bool:
		dst.SetBool(src.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		dst.SetInt(src.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		dst.SetUint(src.Uint())
	case reflect.Float32, reflect.Float64:
		dst.SetFloat(src.Float())
	case reflect.String:
		dst.SetString(src.String())
	default:
		return src.IsZero()
	}
	return true
}
