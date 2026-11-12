package vm

// Native channels made by the program.
//
// make(chan T) of the program made a chan Value: native code that gets it
// sees a closed stand-in (chanstandin.go), not a channel it can wait on.
// context.WithCancel with a Context of the program as the parent waits on
// its Done() channel, and stopped with "passing interpreted channel to
// native code". When T has an exact native form, make now makes a real
// native channel of T, held as a reflect.Value: send, receive, select,
// close, len and cap already work on native channels (chanSend, natRecv,
// execSelect, closeChan), and native code gets the channel itself.
//
// The element types with an exact native form are, for now, struct{} and
// bool without a name of the program: the channels of signals (Done,
// CloseNotify, a quit channel). Their send and receive take a fast path on
// the typed Go channel. A channel of a type of the program stays a chan
// Value.

import (
	"go/types"
	"reflect"
)

var (
	chanStructT = reflect.TypeFor[chan struct{}]()
	chanBoolT   = reflect.TypeFor[chan bool]()
)

// nativeChanType returns the native type of a channel of elem when make
// makes a native channel for it, else nil.
func nativeChanType(elem types.Type) reflect.Type {
	switch u := types.Unalias(elem).(type) { // type E = struct{} is struct{}
	case *types.Struct:
		if u.NumFields() == 0 {
			return chanStructT
		}
	case *types.Basic:
		if u.Kind() == types.Bool {
			return chanBoolT
		}
	}
	return nil
}

// makeNativeChan makes a native channel of type rt with a buffer of n
// elements. A bad size panics as make(chan Value, n) does, as in gc.
func makeNativeChan(rt reflect.Type, n int64) reflect.Value {
	if n < 0 || int64(int(n)) != n {
		_ = make(chan Value, n)
	}
	switch rt {
	case chanStructT:
		return reflect.ValueOf(make(chan struct{}, n))
	case chanBoolT:
		return reflect.ValueOf(make(chan bool, n))
	}
	return reflect.MakeChan(rt, int(n))
}

// fastSend sends v on rv when rv is a chan struct{} or a chan bool, without
// reflect. ok is false for other channels.
func (cx *ctx) fastSend(rv reflect.Value, v Value) (ok bool) {
	switch rv.Type() {
	case chanStructT:
		ch := rv.Interface().(chan struct{})
		select {
		case ch <- struct{}{}:
		default:
			as, ms := cx.stopChans()
			select {
			case ch <- struct{}{}:
			case <-as:
				cx.halted()
			case <-ms:
				cx.halted()
			}
		}
	case chanBoolT:
		ch, b := rv.Interface().(chan bool), v.N != 0
		select {
		case ch <- b:
		default:
			as, ms := cx.stopChans()
			select {
			case ch <- b:
			case <-as:
				cx.halted()
			case <-ms:
				cx.halted()
			}
		}
	default:
		return false
	}
	cx.blocked()
	return true
}

// fastRecv receives from rv when rv is a chan struct{} or a chan bool,
// without reflect: the VM value of the element, and ok of the receive.
// fast is false for other channels.
func (cx *ctx) fastRecv(rv reflect.Value) (v Value, ok, fast bool) {
	switch rv.Type() {
	case chanStructT:
		ch := rv.Interface().(chan struct{})
		select {
		case _, ok = <-ch:
		default:
			as, ms := cx.stopChans()
			select {
			case _, ok = <-ch:
			case <-as:
				cx.halted()
			case <-ms:
				cx.halted()
			}
		}
		v = Value{R: []Value{}} // struct{}: the zero value of zero (no fields)
	case chanBoolT:
		ch := rv.Interface().(chan bool)
		var b bool
		select {
		case b, ok = <-ch:
		default:
			as, ms := cx.stopChans()
			select {
			case b, ok = <-ch:
			case <-as:
				cx.halted()
			case <-ms:
				cx.halted()
			}
		}
		v = Value{N: b2i(b)}
	default:
		return Value{}, false, false
	}
	cx.blocked()
	return v, ok, true
}
