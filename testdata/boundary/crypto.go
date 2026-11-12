// Native boundary of the small crypto packages: crypto, crypto/cipher,
// crypto/ecdsa, crypto/elliptic, crypto/hmac, crypto/ecdh (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"crypto"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math/big"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// short gives the first bytes of b in hex.
func short(b []byte) string {
	if len(b) > 12 {
		b = b[:12]
	}
	return hex.EncodeToString(b)
}

// fixedReader is a deterministic random source: it gives 1, 2, 3...
type fixedReader struct{ n byte }

func (r *fixedReader) Read(p []byte) (int, error) {
	for i := range p {
		r.n++
		p[i] = r.n
	}
	return len(p), nil
}

// edKey is the deterministic Ed25519 key of the probes.
var edKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))

// edSigner is a crypto.Signer (value receivers) without SignMessage.
type edSigner struct{ k ed25519.PrivateKey }

func (s edSigner) Public() crypto.PublicKey { return s.k.Public() }
func (s edSigner) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	out("edSigner.Sign len %d hash %v", len(d), o.HashFunc())
	return s.k.Sign(r, d, o)
}

// msgSigner is a crypto.MessageSigner (pointer receivers).
type msgSigner struct{ k ed25519.PrivateKey }

func (s *msgSigner) Public() crypto.PublicKey { return s.k.Public() }
func (s *msgSigner) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	out("msgSigner.Sign")
	return s.k.Sign(r, d, o)
}
func (s *msgSigner) SignMessage(r io.Reader, m []byte, o crypto.SignerOpts) ([]byte, error) {
	out("msgSigner.SignMessage %q", m)
	return s.k.Sign(r, m, o)
}

// gSigner is a generic crypto.MessageSigner.
type gSigner[T any] struct {
	k   ed25519.PrivateKey
	tag T
}

func (s gSigner[T]) Public() crypto.PublicKey { return s.k.Public() }
func (s gSigner[T]) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	out("gSigner.Sign")
	return s.k.Sign(r, d, o)
}
func (s gSigner[T]) SignMessage(r io.Reader, m []byte, o crypto.SignerOpts) ([]byte, error) {
	out("gSigner[%v].SignMessage %q", s.tag, m)
	return s.k.Sign(r, m, o)
}

// digestSigner records the digest it is given.
type digestSigner struct{}

func (digestSigner) Public() crypto.PublicKey { return nil }
func (digestSigner) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	return []byte("digest:" + hex.EncodeToString(d[:4])), nil
}

// xorBlock is a 16-byte block cipher without the optional methods.
type xorBlock struct{ k byte }

func (b xorBlock) BlockSize() int { return 16 }
func (b xorBlock) Encrypt(dst, src []byte) {
	for i := 0; i < 16; i++ {
		dst[i] = src[i] ^ b.k ^ byte(i)
	}
}
func (b xorBlock) Decrypt(dst, src []byte) { b.Encrypt(dst, src) }

// smallBlock is an 8-byte block cipher.
type smallBlock struct{}

func (smallBlock) BlockSize() int { return 8 }
func (smallBlock) Encrypt(dst, src []byte) {
	copy(dst[:8], src[:8])
}
func (smallBlock) Decrypt(dst, src []byte) { copy(dst[:8], src[:8]) }

// ableBlock has every optional method of crypto/cipher (pointer receivers).
type ableBlock struct{ in xorBlock }

func (b *ableBlock) BlockSize() int          { return 16 }
func (b *ableBlock) Encrypt(dst, src []byte) { b.in.Encrypt(dst, src) }
func (b *ableBlock) Decrypt(dst, src []byte) { b.in.Decrypt(dst, src) }
func (b *ableBlock) NewCBCEncrypter(iv []byte) cipher.BlockMode {
	out("ableBlock.NewCBCEncrypter")
	return cipher.NewCBCEncrypter(b.in, iv)
}
func (b *ableBlock) NewCBCDecrypter(iv []byte) cipher.BlockMode {
	out("ableBlock.NewCBCDecrypter")
	return cipher.NewCBCDecrypter(b.in, iv)
}
func (b *ableBlock) NewCTR(iv []byte) cipher.Stream {
	out("ableBlock.NewCTR")
	return cipher.NewCTR(b.in, iv)
}
func (b *ableBlock) NewGCM(nonceSize, tagSize int) (cipher.AEAD, error) {
	out("ableBlock.NewGCM %d %d", nonceSize, tagSize)
	return cipher.NewGCMWithTagSize(b.in, tagSize)
}

// gBlock is a generic block cipher with the optional methods (value receivers).
type gBlock[T any] struct {
	in  xorBlock
	tag T
}

func (b gBlock[T]) BlockSize() int          { return 16 }
func (b gBlock[T]) Encrypt(dst, src []byte) { b.in.Encrypt(dst, src) }
func (b gBlock[T]) Decrypt(dst, src []byte) { b.in.Decrypt(dst, src) }
func (b gBlock[T]) NewCBCEncrypter(iv []byte) cipher.BlockMode {
	out("gBlock[%v].NewCBCEncrypter", b.tag)
	return cipher.NewCBCEncrypter(b.in, iv)
}
func (b gBlock[T]) NewCBCDecrypter(iv []byte) cipher.BlockMode {
	out("gBlock[%v].NewCBCDecrypter", b.tag)
	return cipher.NewCBCDecrypter(b.in, iv)
}
func (b gBlock[T]) NewCTR(iv []byte) cipher.Stream {
	out("gBlock[%v].NewCTR", b.tag)
	return cipher.NewCTR(b.in, iv)
}
func (b gBlock[T]) NewGCM(nonceSize, tagSize int) (cipher.AEAD, error) {
	out("gBlock[%v].NewGCM %d %d", b.tag, nonceSize, tagSize)
	return nil, fmt.Errorf("gBlock: no GCM")
}

// closeWriter is a writer with Close.
type closeWriter struct{ sb strings.Builder }

func (w *closeWriter) Write(p []byte) (int, error) { return w.sb.Write(p) }
func (w *closeWriter) Close() error                { w.sb.WriteString("[Close]"); return nil }

// plainWriter is a writer without Close.
type plainWriter struct{ sb *strings.Builder }

func (w plainWriter) Write(p []byte) (int, error) { return w.sb.Write(p) }

// ucurve is a curve that cannot be compared (it has a slice).
type ucurve struct {
	elliptic.Curve
	tags []string
}

// ccurve is a curve that can be compared.
type ccurve struct {
	elliptic.Curve
	id int
}

// unCurve is a curve with the Unmarshal methods of elliptic.
type unCurve struct{ elliptic.Curve }

func (c unCurve) Unmarshal(b []byte) (x, y *big.Int) {
	out("unCurve.Unmarshal %d", len(b))
	return big.NewInt(1), big.NewInt(2)
}
func (c unCurve) UnmarshalCompressed(b []byte) (x, y *big.Int) {
	out("unCurve.UnmarshalCompressed %d", len(b))
	return big.NewInt(3), big.NewInt(4)
}

// vhash is a hash.Hash with value receivers over a std hash (comparable).
type vhash struct{ h hash.Hash }

func (v vhash) Write(p []byte) (int, error) { return v.h.Write(p) }
func (v vhash) Sum(b []byte) []byte         { return v.h.Sum(b) }
func (v vhash) Reset()                      { v.h.Reset() }
func (v vhash) Size() int                   { return v.h.Size() }
func (v vhash) BlockSize() int              { return v.h.BlockSize() }

// uhash is a hash.Hash that cannot be compared.
type uhash struct {
	h    hash.Hash
	tags []string
}

func (v uhash) Write(p []byte) (int, error) { return v.h.Write(p) }
func (v uhash) Sum(b []byte) []byte         { return v.h.Sum(b) }
func (v uhash) Reset()                      { v.h.Reset() }
func (v uhash) Size() int                   { return v.h.Size() }
func (v uhash) BlockSize() int              { return v.h.BlockSize() }

// mhash is a hash.Hash with MarshalBinary and UnmarshalBinary.
type mhash struct{ h hash.Hash }

func (v *mhash) Write(p []byte) (int, error) { return v.h.Write(p) }
func (v *mhash) Sum(b []byte) []byte         { return v.h.Sum(b) }
func (v *mhash) Reset()                      { v.h.Reset() }
func (v *mhash) Size() int                   { return v.h.Size() }
func (v *mhash) BlockSize() int              { return v.h.BlockSize() }
func (v *mhash) MarshalBinary() ([]byte, error) {
	marshalCalls++
	return v.h.(interface{ MarshalBinary() ([]byte, error) }).MarshalBinary()
}
func (v *mhash) UnmarshalBinary(b []byte) error {
	unmarshalCalls++
	return v.h.(interface{ UnmarshalBinary([]byte) error }).UnmarshalBinary(b)
}

var marshalCalls, unmarshalCalls int

// ghash is a generic hash.Hash.
type ghash[T any] struct {
	h   hash.Hash
	tag T
}

func (v ghash[T]) Write(p []byte) (int, error) { return v.h.Write(p) }
func (v ghash[T]) Sum(b []byte) []byte         { return v.h.Sum(b) }
func (v ghash[T]) Reset()                      { v.h.Reset() }
func (v ghash[T]) Size() int                   { return v.h.Size() }
func (v ghash[T]) BlockSize() int              { return v.h.BlockSize() }

func main() {
	// row: crypto SignMessage assert x.(crypto.MessageSigner)
	probe("SignMessage", func() {
		signers := []crypto.Signer{edSigner{edKey}, &msgSigner{edKey}, gSigner[int]{edKey, 3}}
		for i, s := range signers {
			sig, err := crypto.SignMessage(s, nil, []byte("msg"), crypto.Hash(0))
			out("%d: %s %v verify %v", i, short(sig), err, ed25519.Verify(edKey.Public().(ed25519.PublicKey), []byte("msg"), sig))
		}
		sig, err := crypto.SignMessage(digestSigner{}, nil, []byte("msg"), crypto.SHA256)
		want := sha256.Sum256([]byte("msg"))
		out("digest: %s %v (want %s)", sig, err, hex.EncodeToString(want[:4]))
		var s crypto.Signer = &msgSigner{edKey}
		_, ok := s.(crypto.MessageSigner)
		out("iface MessageSigner %v", ok)
	})

	key := []byte("0123456789abcdef")
	iv := []byte("fedcba9876543210")
	plain := []byte("plain text of 32 bytes, exactly")
	plain = append(plain, '!')
	// row: crypto/cipher NewCBCEncrypter assert x.(crypto/cipher.cbcEncAble)
	// row: crypto/cipher NewCBCDecrypter assert x.(crypto/cipher.cbcDecAble)
	probe("CBC", func() {
		blocks := []cipher.Block{xorBlock{key[0]}, &ableBlock{xorBlock{key[0]}}, gBlock[string]{xorBlock{key[0]}, "g"}}
		for i, b := range blocks {
			ct := make([]byte, len(plain))
			cipher.NewCBCEncrypter(b, iv).CryptBlocks(ct, plain)
			pt := make([]byte, len(ct))
			cipher.NewCBCDecrypter(b, iv).CryptBlocks(pt, ct)
			out("%d: %s %q", i, short(ct), pt)
		}
	})
	// row: crypto/cipher NewCTR assert x.(crypto/cipher.ctrAble)
	probe("CTR", func() {
		blocks := []cipher.Block{xorBlock{key[1]}, &ableBlock{xorBlock{key[1]}}, gBlock[int]{xorBlock{key[1]}, 5}}
		for i, b := range blocks {
			ct := make([]byte, len(plain))
			cipher.NewCTR(b, iv).XORKeyStream(ct, plain)
			out("%d: %s", i, short(ct))
		}
	})
	// row: crypto/cipher NewGCM,NewGCMWithNonceSize,NewGCMWithTagSize assert x.(crypto/cipher.gcmAble)
	probe("GCM", func() {
		blocks := []cipher.Block{xorBlock{key[2]}, &ableBlock{xorBlock{key[2]}}, gBlock[bool]{xorBlock{key[2]}, true}, smallBlock{}}
		for i, b := range blocks {
			for _, mk := range []struct {
				name string
				f    func(cipher.Block) (cipher.AEAD, error)
			}{
				{"NewGCM", cipher.NewGCM},
				{"NonceSize", func(b cipher.Block) (cipher.AEAD, error) { return cipher.NewGCMWithNonceSize(b, 16) }},
				{"TagSize", func(b cipher.Block) (cipher.AEAD, error) { return cipher.NewGCMWithTagSize(b, 12) }},
			} {
				a, err := mk.f(b)
				if err != nil {
					out("%d %s: error %v", i, mk.name, err)
					continue
				}
				nonce := make([]byte, a.NonceSize())
				ct := a.Seal(nil, nonce, plain, []byte("ad"))
				pt, err := a.Open(nil, nonce, ct, []byte("ad"))
				out("%d %s: nonce %d overhead %d %s %q %v", i, mk.name, a.NonceSize(), a.Overhead(), short(ct), pt, err)
			}
		}
	})
	// row: crypto/cipher StreamWriter.Close assert x.(io.Closer)
	probe("StreamWriter", func() {
		cw := &closeWriter{}
		w := cipher.StreamWriter{S: cipher.NewCTR(xorBlock{1}, iv), W: cw}
		w.Write([]byte("abc"))
		out("closer: %v %d %v", w.Close(), cw.sb.Len(), strings.HasSuffix(cw.sb.String(), "[Close]"))
		sb := &strings.Builder{}
		w = cipher.StreamWriter{S: cipher.NewCTR(xorBlock{1}, iv), W: plainWriter{sb}}
		w.Write([]byte("abc"))
		out("plain: %v %d", w.Close(), sb.Len())
		var iw io.Writer = &closeWriter{}
		w = cipher.StreamWriter{S: cipher.NewCTR(xorBlock{1}, iv), W: iw}
		out("iface: %v %q", w.Close(), iw.(*closeWriter).sb.String())
	})

	ek, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	pub := ek.PublicKey
	// row: crypto/ecdsa (*PublicKey).Equal,(*PrivateKey).Equal panic == of two interfaces: comparing uncomparable type
	probe("ecdsa/Equal/comparable", func() {
		ck := ecdsa.PublicKey{Curve: ccurve{elliptic.P256(), 1}, X: pub.X, Y: pub.Y}
		ck2 := ecdsa.PublicKey{Curve: ccurve{elliptic.P256(), 1}, X: pub.X, Y: pub.Y}
		ck3 := ecdsa.PublicKey{Curve: ccurve{elliptic.P256(), 2}, X: pub.X, Y: pub.Y}
		out("std %v", pub.Equal(&ek.PublicKey))
		out("ccurve same %v, other id %v, vs std %v", ck.Equal(&ck2), ck.Equal(&ck3), ck.Equal(&pub))
		pk := ecdsa.PrivateKey{PublicKey: ck, D: ek.D}
		pk2 := ecdsa.PrivateKey{PublicKey: ck2, D: ek.D}
		out("private ccurve %v, vs std %v", pk.Equal(&pk2), pk.Equal(ek))
	})
	probe("ecdsa/Equal/uncomparable", func() {
		uk := ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: ucurve{elliptic.P256(), nil}, X: pub.X, Y: pub.Y}, D: ek.D}
		out("ucurve vs std pub: %v", uk.PublicKey.Equal(&pub))
		func() {
			defer func() { out("PrivateKey.Equal recovered: %v", recover()) }()
			out("PrivateKey.Equal: %v", uk.Equal(&uk))
		}()
		uk2 := uk
		out("PublicKey.Equal: %v", uk.PublicKey.Equal(&uk2.PublicKey))
	})
	// row: crypto/ecdsa GenerateKey,Sign,SignASN1 assert x.(crypto/internal/fips140/hmac.marshalable)
	// row: crypto/ecdsa GenerateKey,Sign,SignASN1 panic x.(crypto/internal/fips140/hmac.marshalable) without ok: *runtime.TypeAssertionError
	// row: crypto/ecdsa GenerateKey,Sign,SignASN1,(*PrivateKey).Sign panic == of two interfaces: comparing uncomparable type
	// row: crypto/ecdsa (*PrivateKey).Sign assert x.(crypto/internal/fips140/hmac.marshalable)
	// row: crypto/ecdsa (*PrivateKey).Sign panic x.(crypto/internal/fips140/hmac.marshalable) without ok: *runtime.TypeAssertionError
	// row: crypto/ecdsa Sign,Verify,VerifyASN1 assert x.(T), T unnamed
	// row: crypto/ecdsa Sign,Verify,VerifyASN1 reflect walk
	probe("ecdsa/Sign", func() {
		digest := sha256.Sum256([]byte("hello"))
		curves := []elliptic.Curve{elliptic.P256(), ccurve{elliptic.P256(), 1}, ucurve{elliptic.P384(), []string{"x"}}}
		for i, c := range curves {
			k, err := ecdsa.GenerateKey(c, &fixedReader{})
			if err != nil {
				out("%d: GenerateKey error %v", i, err)
				continue
			}
			sig, err := ecdsa.SignASN1(&fixedReader{}, k, digest[:])
			kind := "std"
			switch k.Curve.(type) {
			case ccurve:
				kind = "ccurve"
			case ucurve:
				kind = "ucurve"
			}
			out("%d: curve %s %s, SignASN1 %v verify %v", i, kind, k.Params().Name, err, ecdsa.VerifyASN1(&k.PublicKey, digest[:], sig))
			r, s, err := ecdsa.Sign(&fixedReader{}, k, digest[:])
			out("%d: Sign %v verify %v", i, err, ecdsa.Verify(&k.PublicKey, digest[:], r, s))
			sig, err = k.Sign(&fixedReader{}, digest[:], crypto.SHA256)
			out("%d: (*PrivateKey).Sign %v verify %v", i, err, ecdsa.VerifyASN1(&k.PublicKey, digest[:], sig))
			out("%d: bad verify %v", i, ecdsa.VerifyASN1(&k.PublicKey, digest[:], []byte{0x30, 0}))
		}
	})
	// row: crypto/elliptic Unmarshal,UnmarshalCompressed assert x.(crypto/elliptic.unmarshaler)
	probe("elliptic/Unmarshal", func() {
		p := elliptic.P256().Params()
		data := elliptic.Marshal(elliptic.P256(), p.Gx, p.Gy)
		comp := elliptic.MarshalCompressed(elliptic.P256(), p.Gx, p.Gy)
		curves := []elliptic.Curve{elliptic.P256(), ccurve{elliptic.P256(), 1}, unCurve{elliptic.P256()}}
		for i, c := range curves {
			x, y := elliptic.Unmarshal(c, data)
			out("%d: Unmarshal %v %v", i, x != nil && x.Cmp(p.Gx) == 0, y != nil && y.Cmp(p.Gy) == 0)
			if i == 2 {
				out("%d: values %v %v", i, x, y)
			}
			x, y = elliptic.UnmarshalCompressed(c, comp)
			out("%d: UnmarshalCompressed %v %v", i, x != nil && x.Cmp(p.Gx) == 0, y != nil && y.Cmp(p.Gy) == 0)
			if i == 2 {
				out("%d: values %v %v", i, x, y)
			}
		}
	})
	// row: crypto/hmac New panic == of two interfaces: comparing uncomparable type
	probe("hmac/New", func() {
		fs := []struct {
			name string
			f    func() hash.Hash
		}{
			{"std", sha256.New},
			{"vhash", func() hash.Hash { return vhash{sha256.New()} }},
			{"uhash", func() hash.Hash { return uhash{sha256.New(), []string{"u"}} }},
			{"mhash", func() hash.Hash { return &mhash{sha256.New()} }},
			{"ghash", func() hash.Hash { return ghash[int]{sha256.New(), 1} }},
		}
		for _, f := range fs {
			m := hmac.New(f.f, []byte("key"))
			m.Write([]byte("data"))
			a := m.Sum(nil)
			m.Reset()
			m.Write([]byte("data"))
			b := m.Sum(nil)
			m.Reset()
			m.Write([]byte("data"))
			c := m.Sum(nil)
			out("%s: %s same %v %v", f.name, short(a), bytes.Equal(a, b), bytes.Equal(a, c))
		}
		out("mhash marshal %v unmarshal %v", marshalCalls > 0, unmarshalCalls > 0)
		shared := vhash{sha256.New()}
		func() {
			defer func() { out("same value: recovered %v", recover()) }()
			hmac.New(func() hash.Hash { return shared }, []byte("k"))
		}()
		sharedU := uhash{sha256.New(), nil}
		m := hmac.New(func() hash.Hash { return sharedU }, []byte("k"))
		out("same uncomparable value: size %d", m.Size())
	})
	// row: crypto/ecdh (*PrivateKey).ECDH panic == of two interfaces: comparing uncomparable type
	probe("ecdh/ECDH", func() {
		a, err := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{1}, 32))
		b, err2 := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{2}, 32))
		out("keys %v %v", err, err2)
		s1, err := a.ECDH(b.PublicKey())
		s2, err2 := b.ECDH(a.PublicKey())
		out("x25519: %s %v %v %v", short(s1), bytes.Equal(s1, s2), err, err2)
		p, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			out("error %v", err)
			return
		}
		_, err = a.ECDH(p.PublicKey())
		out("mismatch: %v", err)
		out("equal: %v %v", a.Equal(a), a.PublicKey().Equal(p.PublicKey()))
	})
}
