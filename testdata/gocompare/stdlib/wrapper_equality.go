// Two values of the program that are equal stay equal in native code: the
// wrappers that native code gets hold the same payload. hmac.New refuses a
// hash function that gives the same value twice, and ecdsa.PublicKey.Equal
// compares the curves with ==. But +0 == -0, and the methods of the two
// values must still run on the right one (errors.Join calls them in native
// code).
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"math"
	"math/big"
)

type vhash struct{ hash.Hash }

type ccurve struct {
	elliptic.Curve
	id int
}

type fErr float64

func (e fErr) Error() string { return fmt.Sprint(1 / float64(e)) }

type sErr struct {
	n int
	f float64
}

func (e sErr) Error() string { return fmt.Sprint(e.n, 1/e.f) }

type aErr [2]complex128

func (e aErr) Error() string { return fmt.Sprint(1 / real(e[1])) }

type iErr struct{ x any }

func (e iErr) Error() string { return fmt.Sprint(1 / e.x.(float64)) }

func main() {
	shared := vhash{sha256.New()}
	func() {
		defer func() { fmt.Println("same hash:", recover()) }()
		hmac.New(func() hash.Hash { return shared }, []byte("k"))
	}()
	m := hmac.New(func() hash.Hash { return vhash{sha256.New()} }, []byte("k"))
	m.Write([]byte("data"))
	fmt.Printf("%x\n", m.Sum(nil))

	x, y := big.NewInt(1), big.NewInt(2)
	k1 := ecdsa.PublicKey{Curve: ccurve{elliptic.P256(), 1}, X: x, Y: y}
	k2 := ecdsa.PublicKey{Curve: ccurve{elliptic.P256(), 1}, X: x, Y: y}
	k3 := ecdsa.PublicKey{Curve: ccurve{elliptic.P256(), 2}, X: x, Y: y}
	fmt.Println(k1.Equal(&k2), k1.Equal(&k3))

	pz, nz := 0.0, math.Copysign(0, -1)
	fmt.Println(errors.Join(fErr(pz), fErr(nz)))
	fmt.Println(errors.Join(fErr(nz), fErr(pz)))
	fmt.Println(errors.Join(sErr{1, pz}, sErr{1, nz}))
	fmt.Println(errors.Join(aErr{1, complex(nz, 0)}, aErr{1, complex(pz, 0)}))
	fmt.Println(errors.Join(iErr{pz}, iErr{nz}))
	fmt.Println(errors.Join(fErr(math.NaN()), fErr(math.NaN())))
}
