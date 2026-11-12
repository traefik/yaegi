package vm

// Native decoders and the pointer they fill.
//
// An interpreted value with methods crosses the boundary in an `any` as a
// hand-written wrapper when its method set matches one (stringerW for a
// String method, errorW, jsonMarshalerW...), so that native code can call
// these methods (fmt, net/http). The wrapper is a struct: given as the
// destination of a decoder, json.Unmarshal(data, &v) failed with
// "json: Unmarshal(non-pointer vm.stringerW)" when *T had a String or Error
// method, and gob and xml refused it too. A decoder does not call these
// methods: the destination is converted like a value without methods, a
// native pointer copied in and written back (boxCopy).
//
// Native json does not see interpreted UnmarshalJSON and UnmarshalText
// methods either. For the destination itself, a pointer wrapper calls them
// (jsonUnmarshalerP...). Such methods on nested values (a struct field) are
// still not seen by native json in default mode; the AOT copy sees them.
//
// fmt scanning (fmt.Sscan, Fscanf...), binary.Read and asn1.Unmarshal fill
// their arguments too: fmt.Sscan("2", &c) with a String
// method on c said "type not a pointer: vm.stringerW". For fmt, the
// destinations are the elements of the variadic ...any, and a pointer
// wrapper calls an interpreted Scan method (fmt.Scanner).

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/ssa"
)

type decodeKind uint8

const (
	decodeNone decodeKind = iota
	decodeAny             // a decoder that does not use the json methods (xml, gob)
	decodeJSON            // encoding/json: UnmarshalJSON and UnmarshalText are called
	decodeScan            // fmt scanning: the Scan method (fmt.Scanner) is called
	// encodeAny: an encoder that only reads the value with reflect and
	// calls none of its methods (binary.Write, asn1.Marshal). The value,
	// a pointer or not, is converted like a value without methods.
	encodeAny
)

// decodeDst is the argument of a native call that a decoder fills: its index
// in nativeSite.params (the receiver first for a method), or -1.
type decodeDst struct {
	arg  int
	kind decodeKind
}

var noDecodeDst = decodeDst{arg: -1}

// decodeDsts lists the native decoders, by the name of their ssa function.
var decodeDsts = map[string]decodeDst{
	"encoding/json.Unmarshal":               {1, decodeJSON},
	"(*encoding/json.Decoder).Decode":       {1, decodeJSON},
	"encoding/xml.Unmarshal":                {1, decodeAny},
	"(*encoding/xml.Decoder).Decode":        {1, decodeAny},
	"(*encoding/xml.Decoder).DecodeElement": {1, decodeAny},
	"(*encoding/gob.Decoder).Decode":        {1, decodeAny},
	"encoding/binary.Read":                  {2, decodeAny},
	"encoding/binary.Decode":                {2, decodeAny},
	"encoding/asn1.Unmarshal":               {1, decodeAny},
	"encoding/asn1.UnmarshalWithParams":     {1, decodeAny},
	"encoding/binary.Write":                 {2, encodeAny},
	"encoding/binary.Encode":                {2, encodeAny},
	"encoding/binary.Append":                {2, encodeAny},
	"encoding/binary.Size":                  {0, encodeAny},
	"encoding/asn1.Marshal":                 {0, encodeAny},
	"encoding/asn1.MarshalWithParams":       {0, encodeAny},
	// fmt scanning: the argument is the variadic ...any, each element is
	// a destination (see toNativeW).
	"fmt.Scan":    {0, decodeScan},
	"fmt.Scanln":  {0, decodeScan},
	"fmt.Scanf":   {1, decodeScan},
	"fmt.Sscan":   {1, decodeScan},
	"fmt.Sscanln": {1, decodeScan},
	"fmt.Sscanf":  {2, decodeScan},
	"fmt.Fscan":   {1, decodeScan},
	"fmt.Fscanln": {1, decodeScan},
	"fmt.Fscanf":  {2, decodeScan},
}

// decodeDstMethods is decodeDsts for the methods of native values called
// through an interface.
var decodeDstMethods = map[methodKey]decodeDst{
	{reflect.TypeFor[*json.Decoder](), "Decode"}:       {1, decodeJSON},
	{reflect.TypeFor[*xml.Decoder](), "Decode"}:        {1, decodeAny},
	{reflect.TypeFor[*xml.Decoder](), "DecodeElement"}: {1, decodeAny},
	{reflect.TypeFor[*gob.Decoder](), "Decode"}:        {1, decodeAny},
}

// decodeDstFuncs gives the native function of each package-level entry of
// decodeDsts. A call through a func value (a struct field, a map, a slice,
// an interface: the resty client of gocloak keeps json.Unmarshal in a
// struct field) has no ssa function, only the native func value
// (callNativeValue), so its destination is found by code pointer
// (decodeDstPtrs). Without it the pointer of the program became a hand
// wrapper: "json: Unmarshal(non-pointer vm.stringerW)".
var decodeDstFuncs = map[string]any{
	"encoding/json.Unmarshal":           json.Unmarshal,
	"encoding/xml.Unmarshal":            xml.Unmarshal,
	"encoding/binary.Read":              binary.Read,
	"encoding/binary.Decode":            binary.Decode,
	"encoding/asn1.Unmarshal":           asn1.Unmarshal,
	"encoding/asn1.UnmarshalWithParams": asn1.UnmarshalWithParams,
	"encoding/binary.Write":             binary.Write,
	"encoding/binary.Encode":            binary.Encode,
	"encoding/binary.Append":            binary.Append,
	"encoding/binary.Size":              binary.Size,
	"encoding/asn1.Marshal":             asn1.Marshal,
	"encoding/asn1.MarshalWithParams":   asn1.MarshalWithParams,
	"fmt.Scan":                          fmt.Scan,
	"fmt.Scanln":                        fmt.Scanln,
	"fmt.Scanf":                         fmt.Scanf,
	"fmt.Sscan":                         fmt.Sscan,
	"fmt.Sscanln":                       fmt.Sscanln,
	"fmt.Sscanf":                        fmt.Sscanf,
	"fmt.Fscan":                         fmt.Fscan,
	"fmt.Fscanln":                       fmt.Fscanln,
	"fmt.Fscanf":                        fmt.Fscanf,
}

// decodeDstPtrs is decodeDsts for the native functions called as func
// values, by code pointer (as printFuncPtrs).
var decodeDstPtrs = func() map[uintptr]decodeDst {
	r := map[uintptr]decodeDst{}
	for name, f := range decodeDstFuncs {
		r[reflect.ValueOf(f).Pointer()] = decodeDsts[name]
	}
	return r
}()

// decodeDstOfValue is decodeDstOf for a native func value.
func decodeDstOfValue(fn reflect.Value) decodeDst {
	if d, ok := decodeDstPtrs[fn.Pointer()]; ok {
		return d
	}
	return noDecodeDst
}

func decodeDstOf(f *ssa.Function) decodeDst {
	if d, ok := decodeDsts[f.String()]; ok {
		return d
	}
	return noDecodeDst
}

func decodeDstOfMethod(t reflect.Type, name string) decodeDst {
	if d, ok := decodeDstMethods[methodKey{t, name}]; ok {
		return d
	}
	return noDecodeDst
}

// unmarshalerWrapper returns a pointer wrapper that calls the interpreted
// UnmarshalJSON and UnmarshalText methods of o, the destination of
// encoding/json, when it has them, or its Scan method for fmt scanning.
func (m *Machine) unmarshalerWrapper(o vmObj, dst decodeKind) (reflect.Value, bool) {
	if dst == decodeScan {
		if o.scanMethod() {
			return reflect.ValueOf(&scannerP{o}), true
		}
		return reflect.Value{}, false
	}
	if dst != decodeJSON {
		return reflect.Value{}, false
	}
	js := o.unmarshalMethod("UnmarshalJSON")
	txt := o.unmarshalMethod("UnmarshalText")
	switch {
	case js && txt:
		return reflect.ValueOf(&jsonTextUnmarshalerP{o}), true
	case js:
		return reflect.ValueOf(&jsonUnmarshalerP{o}), true
	case txt:
		return reflect.ValueOf(&textUnmarshalerP{o}), true
	}
	return m.ptrUnmarshalerWrapper(o)
}

// ptrUnmarshalerWrapper is unmarshalerWrapper for o of type **T when *T has
// an interpreted UnmarshalJSON method (anthropic-sdk-go decodes a response
// with json.NewDecoder(r).Decode(&res), res a *Message). json allocates the
// *T when it is nil and calls the method, or stores nil for a JSON null.
func (m *Machine) ptrUnmarshalerWrapper(o vmObj) (reflect.Value, bool) {
	pp, ok := o.iv.T.T.Underlying().(*types.Pointer)
	if !ok {
		return reflect.Value{}, false
	}
	p, ok := pp.Elem().Underlying().(*types.Pointer)
	if !ok {
		return reflect.Value{}, false
	}
	cell, ok := o.iv.V.R.(*Value)
	if !ok || cell == nil {
		return reflect.Value{}, false
	}
	inner := vmObj{m: m, iv: &Iface{T: m.rtypeOf(pp.Elem()), V: *cell}}
	if !inner.unmarshalMethod("UnmarshalJSON") {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(&ptrJSONUnmarshalerP{m: m, cell: cell, rt: inner.iv.T, elem: p.Elem()}), true
}

// ptrJSONUnmarshalerP is the destination of json for a **T (see
// ptrUnmarshalerWrapper): cell holds the *T.
type ptrJSONUnmarshalerP struct {
	m    *Machine
	cell *Value
	rt   *RType
	elem types.Type
}

func (w *ptrJSONUnmarshalerP) UnmarshalJSON(b []byte) error {
	if string(bytes.TrimSpace(b)) == "null" {
		*w.cell = Value{}
		return nil
	}
	if w.cell.R == nil {
		c := new(Value)
		*c = w.m.zero(w.elem)
		*w.cell = Value{R: c}
	}
	return vmObj{m: w.m, iv: &Iface{T: w.rt, V: *w.cell}}.callUnmarshal("UnmarshalJSON", b)
}

// unmarshalMethod reports whether the method name of the boxed value has
// the signature func([]byte) error.
func (o vmObj) unmarshalMethod(name string) bool {
	fn, ok := o.iv.T.Methods[name]
	if !ok || fn.ssa == nil {
		return false
	}
	sig := fn.ssa.Signature
	return sig.Params().Len() == 1 && types.Identical(sig.Params().At(0).Type(), byteSliceT) &&
		sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), errorT)
}

// scanMethod reports whether the boxed value has the method of fmt.Scanner:
// Scan(fmt.ScanState, rune) error.
func (o vmObj) scanMethod() bool {
	fn, ok := o.iv.T.Methods["Scan"]
	if !ok || fn.ssa == nil {
		return false
	}
	sig := fn.ssa.Signature
	if sig.Params().Len() != 2 || sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), errorT) {
		return false
	}
	st, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Named)
	return ok && st.Obj().Pkg() != nil && st.Obj().Pkg().Path() == "fmt" && st.Obj().Name() == "ScanState" &&
		types.Identical(sig.Params().At(1).Type(), types.Typ[types.Int32])
}

// scannerP is the destination of fmt scanning (fmt.Sscan(str, &v)) when v
// has an interpreted Scan method: fmt calls it.
type scannerP struct{ vmObj }

func (w *scannerP) Scan(st fmt.ScanState, verb rune) error {
	err, _ := w.call("Scan", reflect.ValueOf(&st).Elem(), reflect.ValueOf(verb))[0].Interface().(error)
	return err
}

// nilScanDst returns the destination of fmt scanning for nv, the native
// copy of the pointer in o. For a nil pointer to an interpreted named type
// (var c *Color), nv is a nil pointer of another type (*int): fmt would
// write through it (a nil dereference) or name the wrong type. go run says
// "can't scan type: *main.Color", so nilScanP returns that error. A type
// with a Scan method keeps nv: go run calls the method with a nil receiver,
// which panics in the same way.
func nilScanDst(o vmObj, nv reflect.Value) reflect.Value {
	if nv.Kind() != reflect.Pointer || !nv.IsNil() || o.scanMethod() {
		return nv
	}
	if s := typeString(o.iv.T.T); s != nv.Type().String() {
		return reflect.ValueOf(nilScanP{"can't scan type: " + s})
	}
	return nv
}

// notPtrScanDst returns the destination of fmt scanning for o, a value that
// is not a pointer and has no Scan method: fmt refuses it with "type not a
// pointer: T". The native copy is a hand wrapper or another type, so the
// error of fmt would name it (vm.stringerW); notPtrScanDst names the type of
// the program.
func notPtrScanDst(o vmObj) reflect.Value {
	return reflect.ValueOf(nilScanP{"type not a pointer: " + typeString(o.iv.T.T)})
}

// isPlainScanElem reports whether t is an unnamed basic type or []byte: a
// type that the native type switches of fmt scanning name (*int, *[]byte).
func isPlainScanElem(t types.Type) bool {
	if b := basic(t); b != nil {
		return types.Unalias(t) == b && b.Kind() != types.UnsafePointer && b.Info()&types.IsUntyped == 0
	}
	s, ok := types.Unalias(t).(*types.Slice)
	return ok && types.Identical(s.Elem(), types.Typ[types.Byte])
}

// nilScanP is the destination of fmt scanning that fmt refuses, with the
// error of fmt for the type of the program (see nilScanDst, notPtrScanDst).
type nilScanP struct{ msg string }

func (w nilScanP) Scan(fmt.ScanState, rune) error {
	return errors.New(w.msg)
}

func (o vmObj) callUnmarshal(name string, b []byte) error {
	err, _ := o.call(name, reflect.ValueOf(b))[0].Interface().(error)
	return err
}

type jsonUnmarshalerP struct{ vmObj }

func (w *jsonUnmarshalerP) UnmarshalJSON(b []byte) error { return w.callUnmarshal("UnmarshalJSON", b) }

type textUnmarshalerP struct{ vmObj }

func (w *textUnmarshalerP) UnmarshalText(b []byte) error { return w.callUnmarshal("UnmarshalText", b) }

type jsonTextUnmarshalerP struct{ vmObj }

func (w *jsonTextUnmarshalerP) UnmarshalJSON(b []byte) error {
	return w.callUnmarshal("UnmarshalJSON", b)
}

func (w *jsonTextUnmarshalerP) UnmarshalText(b []byte) error {
	return w.callUnmarshal("UnmarshalText", b)
}
