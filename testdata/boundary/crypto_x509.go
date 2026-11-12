// Native boundary of crypto/x509 and crypto/x509/pkix: program types given
// to x509 (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"time"
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

// sum gives the start of the SHA-256 of b in hex.
func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
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

var (
	edKey  = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	edKey2 = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
)

// signCalls counts the calls of the Sign methods of the program keys.
var signCalls int

// vSigner is a crypto.Signer with value receivers.
type vSigner struct{ k ed25519.PrivateKey }

func (s vSigner) Public() crypto.PublicKey { return s.k.Public() }
func (s vSigner) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	signCalls++
	return s.k.Sign(r, d, o)
}

// pSigner is a crypto.Signer with pointer receivers.
type pSigner struct{ k ed25519.PrivateKey }

func (s *pSigner) Public() crypto.PublicKey { return s.k.Public() }
func (s *pSigner) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	signCalls++
	return s.k.Sign(r, d, o)
}

// gSigner is a generic crypto.Signer.
type gSigner[T any] struct {
	k   ed25519.PrivateKey
	tag T
}

func (s gSigner[T]) Public() crypto.PublicKey { return s.k.Public() }
func (s gSigner[T]) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	signCalls++
	return s.k.Sign(r, d, o)
}

// noSigner has no method.
type noSigner struct{ k ed25519.PrivateKey }

// progPub is a public key type of the program.
type progPub struct{ b []byte }

// pubSigner is a crypto.Signer whose public key is a program type.
type pubSigner struct{}

func (pubSigner) Public() crypto.PublicKey { return progPub{[]byte{1}} }
func (pubSigner) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	return nil, fmt.Errorf("pubSigner cannot sign")
}

// S is a named string type of the program.
type S string

// strer is a fmt.Stringer of the program.
type strer struct{ S string }

func (s strer) String() string { return "strer:" + s.S }

// GS is a generic named string type of the program.
type GS[T any] string

var (
	t0 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	tv = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
)

func template() *x509.Certificate {
	return &x509.Certificate{
		Subject:               pkix.Name{CommonName: "test.example", Organization: []string{"go-vm"}},
		DNSNames:              []string{"test.example"},
		NotBefore:             t0,
		NotAfter:              t1,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
}

func main() {
	var caDER []byte
	// row: crypto/x509 CreateCertificate assert x.(crypto.Signer)
	// row: crypto/x509 CreateCertificate assert x.(crypto/x509.privateKey)
	probe("CreateCertificate", func() {
		var g any = gSigner[string]{edKey, "g"}
		keys := []any{edKey, vSigner{edKey}, &pSigner{edKey}, g, noSigner{edKey}, pubSigner{}, []any{vSigner{edKey}}[0]}
		for i, k := range keys {
			signCalls = 0
			der, err := x509.CreateCertificate(&fixedReader{}, template(), template(), edKey.Public(), k)
			if err != nil {
				out("%d: error %v", i, err)
				continue
			}
			out("%d: %s sign calls %d", i, sum(der), signCalls)
			caDER = der
		}
		ca, err := x509.ParseCertificate(caDER)
		if err != nil {
			out("parse: %v", err)
			return
		}
		out("serial %v", ca.SerialNumber)
		// The parent has a public key: it must be the key of the signer.
		leaf := template()
		leaf.IsCA = false
		leaf.SerialNumber = big.NewInt(42)
		_, err = x509.CreateCertificate(nil, leaf, ca, edKey2.Public(), vSigner{edKey2})
		out("wrong parent key: %v", err)
		der, err := x509.CreateCertificate(nil, leaf, ca, edKey2.Public(), &pSigner{edKey})
		out("leaf: %s %v", sum(der), err)
	})
	// row: crypto/x509 ParseCertificate,ParseCertificates,(*Certificate).Verify,(*CertPool).AppendCertsFromPEM,SystemCertPool assert x.(T), T unnamed
	// row: crypto/x509 ParseCertificate,ParseCertificates,(*Certificate).Verify,(*CertPool).AppendCertsFromPEM,SystemCertPool reflect walk
	probe("Parse", func() {
		ca, err := x509.ParseCertificate(caDER)
		if err != nil {
			out("parse: %v", err)
			return
		}
		out("subject %v, dns %v, %v", ca.Subject, ca.DNSNames, ca.PublicKeyAlgorithm)
		certs, err := x509.ParseCertificates(append(append([]byte{}, caDER...), caDER...))
		out("ParseCertificates: %d %v", len(certs), err)
		_, err = x509.ParseCertificate([]byte{0x30, 3, 1, 2, 3})
		out("bad: %v", err)
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
		out("AppendCertsFromPEM: %v", ok)
		chains, err := ca.Verify(x509.VerifyOptions{Roots: pool, CurrentTime: tv, DNSName: "test.example"})
		out("Verify: %d chains %v", len(chains), err)
		_, err = ca.Verify(x509.VerifyOptions{Roots: x509.NewCertPool(), CurrentTime: tv})
		out("Verify unknown: %v", err)
		sys, err := x509.SystemCertPool()
		out("SystemCertPool: %v %v", sys != nil, err)
	})
	// row: crypto/x509 CreateCertificateRequest assert x.(crypto.Signer)
	// row: crypto/x509 ParseCertificateRequest assert x.(T), T unnamed
	// row: crypto/x509 ParseCertificateRequest reflect walk
	probe("CertificateRequest", func() {
		tmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "csr.example"}, DNSNames: []string{"csr.example"}}
		for i, k := range []any{edKey, vSigner{edKey}, &pSigner{edKey}, gSigner[int]{edKey, 1}, noSigner{}} {
			der, err := x509.CreateCertificateRequest(nil, tmpl, k)
			if err != nil {
				out("%d: error %v", i, err)
				continue
			}
			csr, err := x509.ParseCertificateRequest(der)
			if err != nil {
				out("%d: parse %v", i, err)
				continue
			}
			out("%d: %s %v %v", i, sum(der), csr.Subject, csr.CheckSignature())
		}
	})
	// row: crypto/x509 (*Certificate).CreateCRL assert x.(crypto.Signer)
	// row: crypto/x509 ParseRevocationList assert x.(T), T unnamed
	// row: crypto/x509 ParseRevocationList reflect walk
	probe("CreateCRL", func() {
		ca, err := x509.ParseCertificate(caDER)
		if err != nil {
			out("parse: %v", err)
			return
		}
		revoked := []pkix.RevokedCertificate{{SerialNumber: big.NewInt(7), RevocationTime: t0}}
		for i, k := range []any{edKey, vSigner{edKey}, &pSigner{edKey}, gSigner[int]{edKey, 1}, noSigner{}} {
			der, err := ca.CreateCRL(nil, k, revoked, tv, t1)
			if err != nil {
				out("%d: error %v", i, err)
				continue
			}
			rl, err := x509.ParseRevocationList(der)
			if err != nil {
				out("%d: parse %v", i, err)
				continue
			}
			out("%d: %s entries %d check %v", i, sum(der), len(rl.RevokedCertificateEntries), rl.CheckSignatureFrom(ca))
		}
	})
	// row: crypto/x509 ParsePKIXPublicKey assert x.(T), T unnamed
	// row: crypto/x509 ParsePKIXPublicKey reflect walk
	probe("ParsePKIXPublicKey", func() {
		b, err := x509.MarshalPKIXPublicKey(edKey.Public())
		out("marshal %s %v", sum(b), err)
		k, err := x509.ParsePKIXPublicKey(b)
		out("parse %T %v equal %v", k, err, edKey.Public().(ed25519.PublicKey).Equal(k))
		_, err = x509.MarshalPKIXPublicKey(progPub{})
		out("program key: %v", err)
	})

	cn := asn1.ObjectIdentifier{2, 5, 4, 3}
	ou := asn1.ObjectIdentifier{2, 5, 4, 11}
	other := asn1.ObjectIdentifier{1, 2, 3, 4}
	// row: crypto/x509/pkix Name.String,RDNSequence.String assert x.(T), T unnamed
	probe("pkix/String", func() {
		values := []any{"plain", S("named"), GS[int]("generic"), 12, strer{"a,b"}}
		for i, v := range values {
			n := pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{{Type: cn, Value: v}, {Type: other, Value: v}}}
			out("%d: Name %s", i, n.String())
			seq := pkix.RDNSequence{{{Type: ou, Value: v}}, {{Type: other, Value: v}}}
			out("%d: RDNSequence %s", i, seq.String())
		}
	})
	// row: crypto/x509/pkix (*Name).FillFromRDNSequence assert x.(T), T unnamed
	probe("pkix/FillFromRDNSequence", func() {
		seq := pkix.RDNSequence{
			{{Type: cn, Value: S("named")}},
			{{Type: cn, Value: "plain"}},
			{{Type: ou, Value: GS[string]("generic")}},
			{{Type: ou, Value: "unit"}},
		}
		var n pkix.Name
		n.FillFromRDNSequence(&seq)
		out("CN %q OU %q names %d", n.CommonName, n.OrganizationalUnit, len(n.Names))
	})
}
