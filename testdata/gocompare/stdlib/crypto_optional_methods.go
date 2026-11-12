// Optional methods that crypto/cipher, crypto/elliptic and crypto/hmac look
// for on a value of the program: the modes of a cipher.Block, the
// Unmarshal of an elliptic.Curve, and MarshalBinary of a hash.Hash.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"hash"
	"math/big"
)

type block struct{ cipher.Block }

func (b block) NewCBCEncrypter(iv []byte) cipher.BlockMode {
	fmt.Println("program NewCBCEncrypter")
	return cipher.NewCBCEncrypter(b.Block, iv)
}

func (b block) NewCBCDecrypter(iv []byte) cipher.BlockMode {
	fmt.Println("program NewCBCDecrypter")
	return cipher.NewCBCDecrypter(b.Block, iv)
}

func (b block) NewCTR(iv []byte) cipher.Stream {
	fmt.Println("program NewCTR")
	return cipher.NewCTR(b.Block, iv)
}

func (b block) NewGCM(nonceSize, tagSize int) (cipher.AEAD, error) {
	fmt.Println("program NewGCM", nonceSize, tagSize)
	return cipher.NewGCM(b.Block)
}

// plain has no optional method.
type plain struct{ cipher.Block }

type curve struct{ elliptic.Curve }

func (c curve) Unmarshal(data []byte) (*big.Int, *big.Int) {
	fmt.Println("program Unmarshal")
	return elliptic.Unmarshal(c.Curve, data)
}

func (c curve) UnmarshalCompressed(data []byte) (*big.Int, *big.Int) {
	fmt.Println("program UnmarshalCompressed")
	return elliptic.UnmarshalCompressed(c.Curve, data)
}

type mhash struct{ hash.Hash }

var marshals int

func (h *mhash) MarshalBinary() ([]byte, error) {
	marshals++
	return h.Hash.(interface{ MarshalBinary() ([]byte, error) }).MarshalBinary()
}

func (h *mhash) UnmarshalBinary(b []byte) error {
	return h.Hash.(interface{ UnmarshalBinary([]byte) error }).UnmarshalBinary(b)
}

func main() {
	c, _ := aes.NewCipher(make([]byte, 16))
	iv := make([]byte, 16)
	src := []byte("0123456789abcdef")
	for _, b := range []cipher.Block{block{c}, plain{c}} {
		ct := make([]byte, 16)
		cipher.NewCBCEncrypter(b, iv).CryptBlocks(ct, src)
		pt := make([]byte, 16)
		cipher.NewCBCDecrypter(b, iv).CryptBlocks(pt, ct)
		cipher.NewCTR(b, iv).XORKeyStream(ct, src)
		a, err := cipher.NewGCM(b)
		fmt.Printf("%x %q %x %d %v\n", ct[:4], pt, a.Seal(nil, make([]byte, 12), src, nil)[:4], a.Overhead(), err)
	}

	p := elliptic.P256().Params()
	data := elliptic.Marshal(elliptic.P256(), p.Gx, p.Gy)
	comp := elliptic.MarshalCompressed(elliptic.P256(), p.Gx, p.Gy)
	x, y := elliptic.Unmarshal(curve{elliptic.P256()}, data)
	fmt.Println(x.Cmp(p.Gx) == 0, y.Cmp(p.Gy) == 0)
	x, y = elliptic.UnmarshalCompressed(curve{elliptic.P256()}, comp)
	fmt.Println(x.Cmp(p.Gx) == 0, y.Cmp(p.Gy) == 0)

	m := hmac.New(func() hash.Hash { return &mhash{sha256.New()} }, []byte("key"))
	m.Write([]byte("data"))
	s1 := m.Sum(nil)
	m.Reset()
	m.Write([]byte("data"))
	fmt.Printf("%x %v %v\n", s1[:4], bytes.Equal(s1, m.Sum(nil)), marshals > 0)
}
