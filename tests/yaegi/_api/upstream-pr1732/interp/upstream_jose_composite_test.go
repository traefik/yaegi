package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// TestUpstreamJoseComposite covers composite literals of interpreted struct
// types with an interface-typed field, assigned to a destination of that
// interface type (the go-jose JWE shape, crypter.go recipientInfo construction
// valued from a recipientSigInfo type assertion).
//
// When the enclosing assignment is not inlined into the composite literal
// (struct field, dereferenced pointer or binary interface destination), the
// composite literal must store its result concrete in its own frame slot: the
// valueInterface wrapper is added later by the assignment itself. Storing the
// wrapper early panicked with "reflect.Set: value of type interp.valueInterface
// is not assignable to type struct { Xkey []uint8 }".
const upstreamJoseCompositeSrc = `
package main

import "fmt"

type KeyProvider interface {
	GetKey() []byte
}

type Key struct {
	Xkey []byte
}

func (k Key) GetKey() []byte { return k.Xkey }

type recipientInfo struct {
	encryptedKey []byte
	provider     KeyProvider
}

type recipientSigInfo struct {
	encryptedKey []byte
	provider     Key
}

type holder struct {
	gen KeyProvider
}

func main() {
	var i interface{} = []byte("secret")
	kb := i.([]byte)

	// Assignment of a composite literal to an interpreted interface struct
	// field (assignment not inlined by cfg): the reported panic shape.
	var hh holder
	hh.gen = Key{Xkey: kb}
	fmt.Println("field-assign:", string(hh.gen.GetKey()))

	// Same, with the value flowing through an interface variable.
	var h Key
	h = Key{Xkey: kb}
	var p KeyProvider = h
	fmt.Println("selector:", string(p.GetKey()))

	// Interface-typed field of a composite literal valued from a field of a
	// type-asserted struct, as in go-jose crypter.go:
	//   rec := recipientInfo{encryptedKey: m.encryptedKey, header: m.header}
	// with m obtained from m.(recipientSigInfo).
	var mi interface{} = recipientSigInfo{encryptedKey: []byte("ek"), provider: Key{Xkey: []byte("abc")}}
	rsi := mi.(recipientSigInfo)
	rec := recipientInfo{encryptedKey: rsi.encryptedKey, provider: rsi.provider}
	fmt.Println("field:", string(rec.provider.GetKey()), string(rec.encryptedKey))

	// Composite literal element of a slice with an interface field.
	var sp KeyProvider = Key{Xkey: []byte("s1")}
	hs := []recipientInfo{{provider: sp}}
	fmt.Println("slice:", string(hs[0].provider.GetKey()))

	// Map composite literal with interface values.
	var mp KeyProvider = Key{Xkey: []byte("m1")}
	m := map[string]KeyProvider{"a": mp}
	fmt.Println("map:", string(m["a"].GetKey()))

	// Inlined assignment to a plain interface variable destination: the
	// valueInterface wrapper must still be stored by the composite literal.
	var kp KeyProvider
	kp = Key{Xkey: []byte("inline")}
	fmt.Println("inlined:", string(kp.GetKey()))

	// Definition with a composite literal destination-typed interface.
	var np KeyProvider = Key{Xkey: []byte("def")}
	fmt.Println("define:", string(np.GetKey()))

	// Assignment of a composite literal through a dereferenced pointer.
	hp := &recipientInfo{}
	*hp = recipientInfo{encryptedKey: []byte("st"), provider: Key{Xkey: []byte("star")}}
	fmt.Println("star:", string(hp.provider.GetKey()), string(hp.encryptedKey))
}
`

func TestUpstreamJoseComposite(t *testing.T) {
	var stdout bytes.Buffer
	i := interp.New(interp.Options{Stdout: &stdout})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	if _, err := i.Eval(upstreamJoseCompositeSrc); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"field-assign: secret",
		"selector: secret",
		"field: abc ek",
		"slice: s1",
		"map: m1",
		"inlined: inline",
		"define: def",
		"star: star st",
	}, "\n")
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
