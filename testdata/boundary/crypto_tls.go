// Native boundary of crypto/tls: program keys, connections and errors given
// to tls (docs/BOUNDARY.md). The handshakes run in memory; only facts that do
// not depend on random bytes are printed.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// calls counts the calls of the methods of the program keys.
var (
	callsMu sync.Mutex
	calls   = map[string]int{}
)

func record(name string) {
	callsMu.Lock()
	calls[name]++
	callsMu.Unlock()
}

// takeCalls gives the calls since the last time, sorted.
func takeCalls() string {
	callsMu.Lock()
	defer callsMu.Unlock()
	var s []string
	for k, v := range calls {
		s = append(s, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(s)
	calls = map[string]int{}
	return strings.Join(s, " ")
}

// sigKey is a crypto.Signer with value receivers.
type sigKey struct {
	name string
	k    crypto.Signer
}

func (s sigKey) Public() crypto.PublicKey { return s.k.Public() }
func (s sigKey) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	record(s.name + ".Sign")
	return s.k.Sign(r, d, o)
}

// psigKey is a crypto.Signer with pointer receivers.
type psigKey struct {
	name string
	k    crypto.Signer
}

func (s *psigKey) Public() crypto.PublicKey { return s.k.Public() }
func (s *psigKey) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	record(s.name + ".Sign")
	return s.k.Sign(r, d, o)
}

// gsigKey is a generic crypto.Signer.
type gsigKey[T any] struct {
	name string
	k    crypto.Signer
	tag  T
}

func (s gsigKey[T]) Public() crypto.PublicKey { return s.k.Public() }
func (s gsigKey[T]) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	record(fmt.Sprintf("%s[%v].Sign", s.name, s.tag))
	return s.k.Sign(r, d, o)
}

// msgKey is a crypto.MessageSigner.
type msgKey struct {
	name string
	k    crypto.Signer
}

func (s *msgKey) Public() crypto.PublicKey { return s.k.Public() }
func (s *msgKey) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	record(s.name + ".Sign")
	return s.k.Sign(r, d, o)
}
func (s *msgKey) SignMessage(r io.Reader, m []byte, o crypto.SignerOpts) ([]byte, error) {
	record(s.name + ".SignMessage")
	return s.k.Sign(r, m, o)
}

// decKey is a crypto.Decrypter only.
type decKey struct{ k *rsa.PrivateKey }

func (d decKey) Public() crypto.PublicKey { return d.k.Public() }
func (d decKey) Decrypt(r io.Reader, m []byte, o crypto.DecrypterOpts) ([]byte, error) {
	record("decKey.Decrypt")
	return d.k.Decrypt(r, m, o)
}

// sigDecKey is a crypto.Signer and a crypto.Decrypter (pointer receivers).
type sigDecKey struct{ k *rsa.PrivateKey }

func (d *sigDecKey) Public() crypto.PublicKey { return d.k.Public() }
func (d *sigDecKey) Sign(r io.Reader, m []byte, o crypto.SignerOpts) ([]byte, error) {
	record("sigDecKey.Sign")
	return d.k.Sign(r, m, o)
}
func (d *sigDecKey) Decrypt(r io.Reader, m []byte, o crypto.DecrypterOpts) ([]byte, error) {
	record("sigDecKey.Decrypt")
	return d.k.Decrypt(r, m, o)
}

// gdecKey is a generic crypto.Decrypter.
type gdecKey[T any] struct {
	k   *rsa.PrivateKey
	tag T
}

func (d gdecKey[T]) Public() crypto.PublicKey { return d.k.Public() }
func (d gdecKey[T]) Decrypt(r io.Reader, m []byte, o crypto.DecrypterOpts) ([]byte, error) {
	record(fmt.Sprintf("gdecKey[%v].Decrypt", d.tag))
	return d.k.Decrypt(r, m, o)
}

// noKey has no method.
type noKey struct{ n int }

// progPub is a public key of the program.
type progPub struct{ n int }

// pubKey is a crypto.Signer with a public key of the program.
type pubKey struct{}

func (pubKey) Public() crypto.PublicKey { return progPub{1} }
func (pubKey) Sign(r io.Reader, d []byte, o crypto.SignerOpts) ([]byte, error) {
	record("pubKey.Sign")
	return nil, errors.New("pubKey cannot sign")
}

// bufPipe is one direction of memConn: a buffer without size limit.
type bufPipe struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    bytes.Buffer
	closed bool
}

func newBufPipe() *bufPipe {
	p := &bufPipe{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *bufPipe) close() {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
}

// memAddr is the net.Addr of memConn.
type memAddr string

func (a memAddr) Network() string { return "mem" }
func (a memAddr) String() string  { return string(a) }

// memConn is a net.Conn of the program: its writes never block.
type memConn struct {
	r, w *bufPipe
	name string
}

func memPair() (net.Conn, net.Conn) {
	a, b := newBufPipe(), newBufPipe()
	return &memConn{a, b, "server"}, &memConn{b, a, "client"}
}

func (c *memConn) Read(p []byte) (int, error) {
	c.r.mu.Lock()
	defer c.r.mu.Unlock()
	for c.r.buf.Len() == 0 && !c.r.closed {
		c.r.cond.Wait()
	}
	if c.r.buf.Len() == 0 {
		return 0, io.EOF
	}
	return c.r.buf.Read(p)
}

// Write never fails: when the peer is closed, the bytes are dropped, so that
// the order of the two goroutines does not change what is printed.
func (c *memConn) Write(p []byte) (int, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	if !c.w.closed {
		c.w.buf.Write(p)
		c.w.cond.Broadcast()
	}
	return len(p), nil
}

// Close ends both directions: the peer reads what is left, then io.EOF.
func (c *memConn) Close() error {
	c.r.close()
	c.w.close()
	return nil
}

func (c *memConn) LocalAddr() net.Addr                { return memAddr(c.name) }
func (c *memConn) RemoteAddr() net.Addr               { return memAddr("peer-of-" + c.name) }
func (c *memConn) SetDeadline(t time.Time) error      { return nil }
func (c *memConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *memConn) SetWriteDeadline(t time.Time) error { return nil }

// faultConn gives errors of the program in place of reads and writes.
type faultConn struct {
	net.Conn
	mu        sync.Mutex
	readErrs  []error
	writeErrs []error
}

func (f *faultConn) Read(p []byte) (int, error) {
	f.mu.Lock()
	if len(f.readErrs) > 0 {
		e := f.readErrs[0]
		f.readErrs = f.readErrs[1:]
		f.mu.Unlock()
		return 0, e
	}
	f.mu.Unlock()
	return f.Conn.Read(p)
}

func (f *faultConn) Write(p []byte) (int, error) {
	f.mu.Lock()
	if len(f.writeErrs) > 0 {
		e := f.writeErrs[0]
		f.writeErrs = f.writeErrs[1:]
		f.mu.Unlock()
		return 0, e
	}
	f.mu.Unlock()
	return f.Conn.Write(p)
}

// tempErr is a net.Error that is temporary (value receivers).
type tempErr struct{ s string }

func (e tempErr) Error() string   { return "tempErr " + e.s }
func (e tempErr) Timeout() bool   { return false }
func (e tempErr) Temporary() bool { return true }

// timeoutErr is a net.Error that is a timeout (pointer receivers).
type timeoutErr struct{ s string }

func (e *timeoutErr) Error() string   { return "timeoutErr " + e.s }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return false }

// gNetErr is a generic net.Error.
type gNetErr[T any] struct{ v T }

func (e gNetErr[T]) Error() string   { return fmt.Sprintf("gNetErr %v", e.v) }
func (e gNetErr[T]) Timeout() bool   { return true }
func (e gNetErr[T]) Temporary() bool { return true }

// plainErr is an error without the methods of net.Error.
type plainErr struct{ s string }

func (e plainErr) Error() string { return "plainErr " + e.s }

var tv = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

var (
	edKey   = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	edCert  []byte
	rsaKey  *rsa.PrivateKey
	rsaCert []byte
	ecKey   *ecdsa.PrivateKey
	ecCert  []byte
	pool    = x509.NewCertPool()
)

// selfSigned makes a certificate for test.example signed by key.
func selfSigned(pub crypto.PublicKey, key crypto.Signer, serial int64) []byte {
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: "test.example"},
		DNSNames:              []string{"test.example"},
		NotBefore:             time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		panic(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	pool.AddCert(c)
	return der
}

func setup() {
	edCert = selfSigned(edKey.Public(), edKey, 1)
	var err error
	rsaKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	rsaCert = selfSigned(rsaKey.Public(), rsaKey, 2)
	ecKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	ecCert = selfSigned(ecKey.Public(), ecKey, 3)
}

func now() time.Time { return tv }

// serverConfig gives a server configuration with the certificates.
func serverConfig(certs ...tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: certs, Time: now, ClientCAs: pool}
}

// clientConfig gives a client configuration that trusts the certificates.
func clientConfig(certs ...tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: certs, Time: now, RootCAs: pool, ServerName: "test.example"}
}

// state describes a connection state without random values.
func state(cs tls.ConnectionState) string {
	return fmt.Sprintf("%s %s peer %d chains %d", tls.VersionName(cs.Version), tls.CipherSuiteName(cs.CipherSuite), len(cs.PeerCertificates), len(cs.VerifiedChains))
}

// errText gives the text of an error.
func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// session runs a handshake and an exchange between a server and a client.
// mode says how the handshake starts: "explicit" (Handshake),
// "context" (HandshakeContext) or "implicit" (Read and Write).
func session(scfg, ccfg *tls.Config, mode string, pair func() (net.Conn, net.Conn)) {
	a, b := pair()
	srv := tls.Server(a, scfg)
	cli := tls.Client(b, ccfg)
	sdone := make(chan string, 1)
	go func() {
		var err error
		switch mode {
		case "explicit":
			err = srv.Handshake()
		case "context":
			err = srv.HandshakeContext(context.Background())
		}
		if err != nil {
			a.Close()
			sdone <- "handshake: " + errText(err)
			return
		}
		buf := make([]byte, 64)
		n, err := io.ReadAtLeast(srv, buf, 4)
		if err != nil {
			a.Close()
			sdone <- "read: " + errText(err)
			return
		}
		got := string(buf[:n])
		if _, err := srv.Write([]byte("pong:" + got)); err != nil {
			a.Close()
			sdone <- "write: " + errText(err)
			return
		}
		_, eof := srv.Read(buf)
		cerr := srv.Close()
		sdone <- fmt.Sprintf("%s got %q then %v close %v", state(srv.ConnectionState()), got, eof, cerr)
	}()
	cres := func() string {
		var err error
		switch mode {
		case "explicit":
			err = cli.Handshake()
		case "context":
			err = cli.HandshakeContext(context.Background())
		}
		if err != nil {
			b.Close()
			return "handshake: " + errText(err)
		}
		if _, err := cli.Write([]byte("ping")); err != nil {
			b.Close()
			return "write: " + errText(err)
		}
		buf := make([]byte, 64)
		n, err := io.ReadAtLeast(cli, buf, 9)
		if err != nil {
			b.Close()
			return "read: " + errText(err)
		}
		werr := cli.CloseWrite()
		_, eof := cli.Read(buf)
		cerr := cli.Close()
		return fmt.Sprintf("%s got %q closeWrite %v then %v close %v", state(cli.ConnectionState()), buf[:n], werr, eof, cerr)
	}()
	select {
	case s := <-sdone:
		out("  client: %s", cres)
		out("  server: %s", s)
	case <-time.After(20 * time.Second):
		out("  client: %s", cres)
		out("  server: timeout")
	}
	out("  calls: %s", takeCalls())
}

func main() {
	setup()
	edLeaf := func(k crypto.PrivateKey) tls.Certificate {
		return tls.Certificate{Certificate: [][]byte{edCert}, PrivateKey: k}
	}
	rsaLeaf := func(k crypto.PrivateKey) tls.Certificate {
		return tls.Certificate{Certificate: [][]byte{rsaCert}, PrivateKey: k}
	}
	ecLeaf := func(k crypto.PrivateKey) tls.Certificate {
		return tls.Certificate{Certificate: [][]byte{ecCert}, PrivateKey: k}
	}

	// row: crypto/tls Client,Server,(*Conn).Read assert x.(crypto.Signer)
	// row: crypto/tls Client,Server,(*Conn).Read assert x.(hash.Cloner)
	// row: crypto/tls Client,Server,(*Conn).Read assert x.(crypto/tls.binaryMarshaler)
	// row: crypto/tls Client,Server,(*Conn).Read assert x.(crypto/internal/fips140/hmac.marshalable)
	// row: crypto/tls Client,Server,(*Conn).Read panic x.(crypto/internal/fips140/hmac.marshalable) without ok: *runtime.TypeAssertionError
	// row: crypto/tls Client,Server,(*Conn).Read panic == of two interfaces: comparing uncomparable type
	// row: crypto/tls Client,Server,(*Conn).Handshake,(*Conn).HandshakeContext,(*Conn).Read,(*Conn).Write,(*Conn).CloseWrite,(*Conn).Close assert x.(crypto/cipher.AEAD)
	// row: crypto/tls Client,Server,(*Conn).Handshake,(*Conn).HandshakeContext,(*Conn).Read,(*Conn).Write,(*Conn).CloseWrite,(*Conn).Close assert x.(T), T unnamed
	// row: crypto/tls Client,Server,(*Conn).Handshake,(*Conn).HandshakeContext,(*Conn).Read,(*Conn).Write,(*Conn).CloseWrite,(*Conn).Close panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("TLS13", func() {
		var g crypto.Signer = gsigKey[int]{"gsrv", edKey, 1}
		keys := []crypto.PrivateKey{edKey, sigKey{"srv", edKey}, &psigKey{"psrv", edKey}, g, &msgKey{"msrv", edKey}}
		ckeys := []crypto.PrivateKey{edKey, &psigKey{"pcli", edKey}, gsigKey[string]{"gcli", edKey, "c"}, sigKey{"cli", edKey}, &msgKey{"mcli", edKey}}
		modes := []string{"explicit", "context", "implicit", "explicit", "implicit"}
		for i := range keys {
			scfg := serverConfig(edLeaf(keys[i]))
			scfg.ClientAuth = tls.RequireAndVerifyClientCert
			ccfg := clientConfig(edLeaf(ckeys[i]))
			out("%d: %s", i, modes[i])
			session(scfg, ccfg, modes[i], memPair)
		}
		out("net.Pipe:")
		session(serverConfig(edLeaf(sigKey{"srv", edKey})), clientConfig(), "explicit", net.Pipe)
	})
	// row: crypto/tls Client,Server,(*Conn).Read panic x.(crypto.Signer) without ok: *runtime.TypeAssertionError
	probe("TLS13/BadKeys", func() {
		out("server noKey:")
		session(serverConfig(edLeaf(noKey{1})), clientConfig(), "explicit", memPair)
		out("server pubKey:")
		session(serverConfig(edLeaf(pubKey{})), clientConfig(), "explicit", memPair)
		out("server value ecdsa.PrivateKey:")
		session(serverConfig(ecLeaf(*ecKey)), clientConfig(), "explicit", memPair)
		out("client noKey:")
		scfg := serverConfig(edLeaf(edKey))
		scfg.ClientAuth = tls.RequireAnyClientCert
		session(scfg, clientConfig(edLeaf(noKey{2})), "explicit", memPair)
		out("client GetClientCertificate noKey:")
		ccfg := clientConfig()
		ccfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			c := edLeaf(noKey{3})
			return &c, nil
		}
		session(scfg, ccfg, "explicit", memPair)
	})
	// row: crypto/tls Client,Server,(*Conn).Handshake,(*Conn).HandshakeContext,(*Conn).Read,(*Conn).Write,(*Conn).CloseWrite,(*Conn).Close assert x.(crypto/tls.cbcMode)
	probe("TLS12", func() {
		suites := []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA}
		for i, s := range suites {
			scfg := serverConfig(edLeaf(sigKey{"srv", edKey}), ecLeaf(&psigKey{"ecsrv", ecKey}))
			scfg.ClientAuth = tls.RequireAndVerifyClientCert
			ccfg := clientConfig(edLeaf(&psigKey{"pcli", edKey}))
			ccfg.MaxVersion = tls.VersionTLS12
			ccfg.CipherSuites = []uint16{s}
			out("%d: %s", i, tls.CipherSuiteName(s))
			session(scfg, ccfg, []string{"explicit", "context", "implicit", "explicit"}[i], memPair)
		}
		out("ecdsa client key:")
		scfg := serverConfig(ecLeaf(sigKey{"ecsrv", ecKey}))
		scfg.ClientAuth = tls.RequireAndVerifyClientCert
		ccfg := clientConfig(ecLeaf(gsigKey[int]{"eccli", ecKey, 2}))
		ccfg.MaxVersion = tls.VersionTLS12
		session(scfg, ccfg, "explicit", memPair)
	})
	// row: crypto/tls Server assert x.(crypto.Decrypter)
	// row: crypto/tls Client,Server,(*Conn).Handshake,(*Conn).HandshakeContext,(*Conn).Read,(*Conn).Write,(*Conn).CloseWrite,(*Conn).Close assert x.(crypto/cipher.Stream)
	probe("TLS12/RSAKex", func() {
		suites := []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_RC4_128_SHA}
		keys := []crypto.PrivateKey{decKey{rsaKey}, &sigDecKey{rsaKey}, gdecKey[int]{rsaKey, 4}}
		for i, s := range suites {
			for j, k := range keys {
				// The first certificate cannot be used: Server picks the next one.
				scfg := serverConfig(edLeaf(noKey{4}), rsaLeaf(k))
				scfg.CipherSuites = suites
				scfg.MaxVersion = tls.VersionTLS12
				ccfg := clientConfig()
				ccfg.CipherSuites = []uint16{s}
				mode := []string{"explicit", "context", "implicit"}[j]
				out("%d.%d: %s %s", i, j, tls.CipherSuiteName(s), mode)
				session(scfg, ccfg, mode, memPair)
			}
		}
		out("signer only:")
		scfg := serverConfig(rsaLeaf(sigKey{"rsasrv", rsaKey}))
		scfg.CipherSuites = suites
		ccfg := clientConfig()
		ccfg.CipherSuites = suites
		ccfg.MaxVersion = tls.VersionTLS12
		session(scfg, ccfg, "explicit", memPair)
	})
	// row: crypto/tls (*Conn).Write assert x.(crypto/cipher.BlockMode)
	probe("TLS10", func() {
		for i, k := range []crypto.PrivateKey{decKey{rsaKey}, sigKey{"rsasrv", rsaKey}} {
			scfg := serverConfig(rsaLeaf(k))
			scfg.MinVersion = tls.VersionTLS10
			scfg.MaxVersion = tls.VersionTLS10
			scfg.CipherSuites = []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA}
			ccfg := clientConfig()
			ccfg.MinVersion = tls.VersionTLS10
			ccfg.CipherSuites = scfg.CipherSuites
			out("%d:", i)
			session(scfg, ccfg, "implicit", memPair)
		}
	})
	// row: crypto/tls (*ClientHelloInfo).SupportsCertificate,(*CertificateRequestInfo).SupportsCertificate assert x.(crypto.Signer)
	// row: crypto/tls (*ClientHelloInfo).SupportsCertificate assert x.(crypto.Decrypter)
	probe("SupportsCertificate", func() {
		var gs crypto.Signer = gsigKey[int]{"g", ecKey, 1}
		certs := []struct {
			name string
			c    tls.Certificate
		}{
			{"ed std", edLeaf(edKey)},
			{"ed sigKey", edLeaf(sigKey{"s", edKey})},
			{"ed noKey", edLeaf(noKey{5})},
			{"ed pubKey", edLeaf(pubKey{})},
			{"ec psigKey", ecLeaf(&psigKey{"p", ecKey})},
			{"ec gsigKey", ecLeaf(gs)},
			{"ec value key", ecLeaf(*ecKey)},
			{"rsa decKey", rsaLeaf(decKey{rsaKey})},
			{"rsa sigDecKey", rsaLeaf(&sigDecKey{rsaKey})},
			{"rsa gdecKey", rsaLeaf(gdecKey[string]{rsaKey, "d"})},
			{"rsa sigKey", rsaLeaf(sigKey{"r", rsaKey})},
		}
		infos := []struct {
			name string
			chi  *tls.ClientHelloInfo
		}{
			{"tls13", &tls.ClientHelloInfo{
				CipherSuites:      []uint16{tls.TLS_AES_128_GCM_SHA256},
				SupportedVersions: []uint16{tls.VersionTLS13},
				SignatureSchemes:  []tls.SignatureScheme{tls.Ed25519, tls.ECDSAWithP256AndSHA256, tls.PSSWithSHA256},
				SupportedCurves:   []tls.CurveID{tls.X25519, tls.CurveP256},
				SupportedPoints:   []uint8{0},
			}},
			{"tls12", &tls.ClientHelloInfo{
				CipherSuites:      []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_RSA_WITH_AES_128_GCM_SHA256},
				SupportedVersions: []uint16{tls.VersionTLS12},
				SignatureSchemes:  []tls.SignatureScheme{tls.Ed25519, tls.ECDSAWithP256AndSHA256, tls.PSSWithSHA256},
				SupportedCurves:   []tls.CurveID{tls.X25519, tls.CurveP256},
				SupportedPoints:   []uint8{0},
			}},
			{"tls12 no ecdhe", &tls.ClientHelloInfo{
				CipherSuites:      []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256},
				SupportedVersions: []uint16{tls.VersionTLS12},
				SignatureSchemes:  []tls.SignatureScheme{tls.PKCS1WithSHA256},
			}},
		}
		for _, in := range infos {
			for _, c := range certs {
				out("%s / %s: %v", in.name, c.name, in.chi.SupportsCertificate(&c.c))
			}
		}
		cri := &tls.CertificateRequestInfo{SignatureSchemes: []tls.SignatureScheme{tls.Ed25519, tls.ECDSAWithP256AndSHA256}, Version: tls.VersionTLS13}
		for _, c := range certs {
			out("cri / %s: %v", c.name, cri.SupportsCertificate(&c.c))
		}
		// In a handshake, the ClientHelloInfo has the configuration of the
		// server, with the RSA key exchange.
		out("in GetCertificate:")
		scfg := serverConfig()
		scfg.CipherSuites = []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256}
		scfg.MaxVersion = tls.VersionTLS12
		scfg.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			var pick *tls.Certificate
			for _, c := range certs {
				err := chi.SupportsCertificate(&c.c)
				out("  %s: %v", c.name, err)
				if err == nil && pick == nil {
					c := c
					pick = &c.c
				}
			}
			return pick, nil
		}
		ccfg := clientConfig()
		ccfg.CipherSuites = scfg.CipherSuites
		session(scfg, ccfg, "explicit", memPair)
		out("in GetClientCertificate:")
		scfg = serverConfig(edLeaf(edKey))
		scfg.ClientAuth = tls.RequireAndVerifyClientCert
		ccfg = clientConfig()
		ccfg.GetClientCertificate = func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			var pick *tls.Certificate
			for _, c := range certs {
				err := cri.SupportsCertificate(&c.c)
				out("  %s: %v", c.name, err)
				if err == nil && pick == nil {
					c := c
					pick = &c.c
				}
			}
			return pick, nil
		}
		session(scfg, ccfg, "explicit", memPair)
	})
	// row: crypto/tls Client,Server,(*Conn).Handshake,(*Conn).HandshakeContext,(*Conn).Read,(*Conn).Write,(*Conn).CloseWrite,(*Conn).Close assert x.(net.Error)
	probe("NetError", func() {
		errs := []error{tempErr{"r"}, &timeoutErr{"r"}, gNetErr[int]{7}, plainErr{"r"}}
		for i, e := range errs {
			a, b := memPair()
			fb := &faultConn{Conn: b}
			srv := tls.Server(a, serverConfig(edLeaf(edKey)))
			cli := tls.Client(fb, clientConfig())
			sdone := make(chan error, 1)
			go func() {
				err := srv.Handshake()
				if err == nil {
					_, err = srv.Write([]byte("data"))
				}
				sdone <- err
			}()
			err := cli.Handshake()
			out("%d: handshake %v, server %v", i, err, <-sdone)
			fb.mu.Lock()
			fb.readErrs = []error{e}
			fb.mu.Unlock()
			buf := make([]byte, 16)
			n, err := cli.Read(buf)
			out("%d: read 1: %d %v", i, n, err)
			n, err = cli.Read(buf)
			out("%d: read 2: %d %q %v", i, n, buf[:n], err)
			if ne, ok := err.(net.Error); ok {
				out("%d: read 2 is net.Error: timeout %v temporary %v", i, ne.Timeout(), ne.Temporary())
			}
			out("%d: read 2 errors.Is %v", i, errors.Is(err, e))
			srv.Close()
			a.Close()
		}
		for i, e := range errs {
			a, b := memPair()
			fb := &faultConn{Conn: b}
			srv := tls.Server(a, serverConfig(edLeaf(edKey)))
			cli := tls.Client(fb, clientConfig())
			go srv.Handshake()
			err := cli.Handshake()
			fb.mu.Lock()
			fb.writeErrs = []error{e}
			fb.mu.Unlock()
			n, err := cli.Write([]byte("x"))
			out("%d: write 1: %d %v", i, n, err)
			n, err = cli.Write([]byte("y"))
			out("%d: write 2: %d %v", i, n, err)
			if ne, ok := err.(net.Error); ok {
				out("%d: write 2 is net.Error: timeout %v temporary %v", i, ne.Timeout(), ne.Temporary())
			}
			out("%d: write 2 errors.Is %v, close %v", i, errors.Is(err, e), cli.Close())
			a.Close()
		}
		// An error in the handshake itself.
		for i, e := range errs {
			a, b := memPair()
			fb := &faultConn{Conn: b, readErrs: []error{e}}
			srv := tls.Server(a, serverConfig(edLeaf(edKey)))
			cli := tls.Client(fb, clientConfig())
			go srv.Handshake()
			err := cli.Handshake()
			out("%d: handshake: %v", i, err)
			err = cli.Handshake()
			out("%d: handshake again: %v", i, err)
			a.Close()
		}
	})
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).Start,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket assert x.(T), T unnamed
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).Start,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).Start,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket assert x.(crypto/cipher.AEAD)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).Start,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket assert x.(crypto/cipher.Stream)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).Start,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket assert x.(crypto/tls.cbcMode)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).Start,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket assert x.(net.Error)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket assert x.(crypto/internal/fips140/hmac.marshalable)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket panic x.(crypto/internal/fips140/hmac.marshalable) without ok: *runtime.TypeAssertionError
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData,(*QUICConn).SendSessionTicket panic == of two interfaces: comparing uncomparable type
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData assert x.(crypto.Signer)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData panic x.(crypto.Signer) without ok: *runtime.TypeAssertionError
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData assert x.(hash.Cloner)
	// row: crypto/tls QUICClient,QUICServer,(*QUICConn).HandleData assert x.(crypto/tls.binaryMarshaler)
	// row: crypto/tls QUICServer assert x.(crypto.Decrypter)
	probe("QUIC", func() {
		runs := []struct {
			name       string
			scert      []tls.Certificate
			ccert      []tls.Certificate
			clientAuth tls.ClientAuthType
		}{
			{"signers", []tls.Certificate{edLeaf(sigKey{"qsrv", edKey})}, []tls.Certificate{edLeaf(&psigKey{"qcli", edKey})}, tls.RequireAndVerifyClientCert},
			{"generic and message signer", []tls.Certificate{edLeaf(noKey{6}), rsaLeaf(decKey{rsaKey}), edLeaf(gsigKey[int]{"gq", edKey, 9})}, []tls.Certificate{edLeaf(&msgKey{"mq", edKey})}, tls.RequireAndVerifyClientCert},
			{"server noKey", []tls.Certificate{edLeaf(noKey{7})}, nil, tls.NoClientCert},
			{"client noKey", []tls.Certificate{edLeaf(edKey)}, []tls.Certificate{edLeaf(noKey{8})}, tls.RequireAnyClientCert},
		}
		for _, r := range runs {
			scfg := serverConfig(r.scert...)
			scfg.MinVersion = tls.VersionTLS13
			scfg.ClientAuth = r.clientAuth
			ccfg := clientConfig(r.ccert...)
			ccfg.MinVersion = tls.VersionTLS13
			out("%s:", r.name)
			quicRun(scfg, ccfg)
			out("  calls: %s", takeCalls())
		}
	})
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(crypto.Signer)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext panic x.(crypto.Signer) without ok: *runtime.TypeAssertionError
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(hash.Cloner)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(crypto/tls.binaryMarshaler)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(crypto/internal/fips140/hmac.marshalable)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext panic x.(crypto/internal/fips140/hmac.marshalable) without ok: *runtime.TypeAssertionError
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext panic == of two interfaces: comparing uncomparable type
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(crypto/cipher.AEAD)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(crypto/cipher.Stream)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(crypto/tls.cbcMode)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(net.Error)
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext assert x.(T), T unnamed
	// row: crypto/tls Dial,DialWithDialer,(*Dialer).Dial,(*Dialer).DialContext panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Dial", func() {
		dir, err := os.MkdirTemp("", "tlsdial")
		if err != nil {
			out("MkdirTemp: %v", err)
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "s")
		ln, err := net.Listen("unix", path)
		if err != nil {
			out("listen failed (no unix socket here)")
			return
		}
		defer ln.Close()
		scfg := serverConfig(edLeaf(sigKey{"dsrv", edKey}), rsaLeaf(decKey{rsaKey}))
		scfg.ClientAuth = tls.VerifyClientCertIfGiven
		scfg.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_RSA_WITH_RC4_128_SHA}
		var silent []net.Conn
		var silentMu sync.Mutex
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					// A client that sends "hang" first gets no answer.
					s := tls.Server(c, scfg)
					buf := make([]byte, 16)
					n, err := s.Read(buf)
					if err != nil {
						c.Close()
						return
					}
					s.Write([]byte("pong:" + string(buf[:n])))
					s.Close()
				}()
			}
		}()
		hangPath := filepath.Join(dir, "h")
		hln, err := net.Listen("unix", hangPath)
		if err != nil {
			out("listen failed (no unix socket here)")
			return
		}
		defer hln.Close()
		go func() {
			for {
				c, err := hln.Accept()
				if err != nil {
					return
				}
				silentMu.Lock()
				silent = append(silent, c)
				silentMu.Unlock()
			}
		}()
		use := func(name string, c *tls.Conn, err error) {
			if err != nil {
				out("%s: %v", name, err)
				return
			}
			c.Write([]byte("ping"))
			b, rerr := io.ReadAll(c)
			out("%s: %s %q %v", name, state(c.ConnectionState()), b, rerr)
			c.Close()
			out("%s: calls %s", name, takeCalls())
		}
		cfg := func(max uint16, suite uint16, certs ...tls.Certificate) *tls.Config {
			c := clientConfig(certs...)
			c.MaxVersion = max
			if suite != 0 {
				c.CipherSuites = []uint16{suite}
			}
			return c
		}
		c, err := tls.Dial("unix", path, cfg(0, 0, edLeaf(&psigKey{"dcli", edKey})))
		use("Dial tls13", c, err)
		c, err = tls.Dial("unix", path, cfg(tls.VersionTLS12, tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, edLeaf(sigKey{"dcli", edKey})))
		use("Dial tls12 cbc", c, err)
		c, err = tls.Dial("unix", path, cfg(0, 0, edLeaf(noKey{9})))
		use("Dial noKey", c, err)
		c, err = tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "unix", path, cfg(tls.VersionTLS12, tls.TLS_RSA_WITH_RC4_128_SHA))
		use("DialWithDialer rc4", c, err)
		d := &tls.Dialer{Config: cfg(tls.VersionTLS12, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, edLeaf(gsigKey[int]{"gd", edKey, 3}))}
		nc, err := d.Dial("unix", path)
		tc, _ := nc.(*tls.Conn)
		use("Dialer.Dial", tc, err)
		d = &tls.Dialer{Config: cfg(0, 0, edLeaf(&msgKey{"md", edKey}))}
		nc, err = d.DialContext(context.Background(), "unix", path)
		tc, _ = nc.(*tls.Conn)
		use("Dialer.DialContext", tc, err)
		// A server that never answers: the handshake times out.
		_, err = tls.DialWithDialer(&net.Dialer{Timeout: 200 * time.Millisecond}, "unix", hangPath, cfg(0, 0))
		var ne net.Error
		out("DialWithDialer timeout: %v, net.Error %v timeout %v", err, errors.As(err, &ne), ne != nil && ne.Timeout())
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_, err = (&tls.Dialer{Config: cfg(0, 0)}).DialContext(ctx, "unix", hangPath)
		cancel()
		out("DialContext timeout: %v, deadline %v", err, errors.Is(err, context.DeadlineExceeded))
		silentMu.Lock()
		for _, c := range silent {
			c.Close()
		}
		silentMu.Unlock()
	})
}

// quicRun runs a QUIC handshake in memory and prints what happened.
func quicRun(scfg, ccfg *tls.Config) {
	ctx := context.Background()
	cli := tls.QUICClient(&tls.QUICConfig{TLSConfig: ccfg})
	srv := tls.QUICServer(&tls.QUICConfig{TLSConfig: scfg})
	cli.SetTransportParameters([]byte("cli-params"))
	srv.SetTransportParameters([]byte("srv-params"))
	defer cli.Close()
	defer srv.Close()
	if err := cli.Start(ctx); err != nil {
		out("  client Start: %v", err)
		return
	}
	if err := srv.Start(ctx); err != nil {
		out("  server Start: %v", err)
		return
	}
	conns := [2]*tls.QUICConn{cli, srv}
	names := [2]string{"client", "server"}
	var done [2]bool
	var params [2]string
	a, idle, stop := 0, 0, false
	for steps := 0; steps < 1000 && idle < 2 && !stop; steps++ {
		e := conns[a].NextEvent()
		switch e.Kind {
		case tls.QUICNoEvent:
			idle++
			a = 1 - a
			continue
		case tls.QUICWriteData:
			if err := conns[1-a].HandleData(e.Level, e.Data); err != nil {
				out("  %s HandleData: %v", names[1-a], err)
				stop = true
			}
		case tls.QUICTransportParameters:
			params[a] = string(e.Data)
		case tls.QUICHandshakeDone:
			done[a] = true
			if a == 1 {
				out("  SendSessionTicket: %v", srv.SendSessionTicket(tls.QUICSessionTicketOptions{}))
			}
		case tls.QUICErrorEvent:
			out("  %s error: %v", names[a], e.Err)
			stop = true
		}
		idle = 0
	}
	out("  done %v params %q", done, params)
	if done[0] {
		out("  client: %s", state(cli.ConnectionState()))
		out("  server: %s", state(srv.ConnectionState()))
	}
}
