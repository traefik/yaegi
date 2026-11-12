package joseplug

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// state signs and encrypts tokens with go-jose: JWS (ES256), JWE (RSA-OAEP
// and A256GCM), a nested signed then encrypted JWT, and a JWK set.
type state struct {
	ec      *ecdsa.PrivateKey
	rsa     *rsa.PrivateKey
	signer  jose.Signer
	enc     jose.Encrypter
	jwksLen int
}

// Claims are private claims of the plugin.
type Claims struct {
	Path   string   `json:"path"`
	Method string   `json:"method"`
	Roles  []string `json:"roles,omitempty"`
}

// The keys are fixed so that the outputs do not change between runs.
func fixedEC() *ecdsa.PrivateKey {
	d, _ := new(big.Int).SetString("c6ef9c5d78ae012a011164acb397ce2088685d8f06bf9be0b283ab46476bee53", 16)
	k := &ecdsa.PrivateKey{D: d}
	k.Curve = elliptic.P256()
	k.X, k.Y = k.Curve.ScalarBaseMult(d.Bytes())
	return k
}

func fixedRSA() (*rsa.PrivateKey, error) {
	// Two primes of 1024 bits, test values only.
	p, _ := new(big.Int).SetString("f7e75fdc469067ffdc4e847c51f452df4b2e3f4e9c3b1a0bb6a3d5bfb51de2c6a24f9a5e3b1c6a0f6f1e5d8c7b4f1b2a1d0e9c8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e0d9", 16)
	q, _ := new(big.Int).SetString("d1a4f6c8e0b2d4f6a8c0e2b4d6f8a0c2e4b6d8f0a2c4e6b8d0f2a4c6e8b0d2f4a6c8e0b2d4f6a8c0e2b4d6f8a0c2e4b6d8f0a2c4e6b8d0f2a4c6e8b0d2f4a6c8e0b2d4f6a8c0e2b4d6f8a0c2e4b6d8f0a2c4e6b8d0f2a4c6e8b0d2f4a6c8e0b2d4f6a8c0e2b4d6f8a0c2e4b6d8f0a2c4e6b8d0f2a4c6e8b0d3", 16)
	p = nextPrime(p)
	q = nextPrime(q)
	n := new(big.Int).Mul(p, q)
	e := big.NewInt(65537)
	phi := new(big.Int).Mul(new(big.Int).Sub(p, big.NewInt(1)), new(big.Int).Sub(q, big.NewInt(1)))
	d := new(big.Int).ModInverse(e, phi)
	if d == nil {
		return nil, fmt.Errorf("no inverse")
	}
	k := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: n, E: 65537}, D: d, Primes: []*big.Int{p, q}}
	k.Precompute()
	return k, k.Validate()
}

func nextPrime(n *big.Int) *big.Int {
	n = new(big.Int).SetBit(n, 0, 1)
	for !n.ProbablyPrime(20) {
		n.Add(n, big.NewInt(2))
	}
	return n
}

func setup(_ context.Context) (*state, error) {
	s := &state{ec: fixedEC()}
	var err error
	if s.rsa, err = fixedRSA(); err != nil {
		return nil, err
	}
	s.signer, err = jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: jose.JSONWebKey{Key: s.ec, KeyID: "ec1"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return nil, err
	}
	s.enc, err = jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.RSA_OAEP_256, Key: &s.rsa.PublicKey, KeyID: "rsa1"},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))
	if err != nil {
		return nil, err
	}
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &s.ec.PublicKey, KeyID: "ec1", Algorithm: "ES256", Use: "sig"},
		{Key: &s.rsa.PublicKey, KeyID: "rsa1", Algorithm: "RSA-OAEP-256", Use: "enc"},
	}}
	b, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	var back jose.JSONWebKeySet
	if err := json.Unmarshal(b, &back); err != nil {
		return nil, err
	}
	s.jwksLen = len(back.Key("ec1")) + len(back.Key("rsa1"))
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	std := jwt.Claims{Issuer: "traefik", Subject: req.Host, Audience: jwt.Audience{"plugin"},
		IssuedAt: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(time.Hour))}
	priv := Claims{Path: req.URL.Path, Method: req.Method, Roles: []string{"user"}}

	signed, err := jwt.Signed(s.signer).Claims(std).Claims(priv).Serialize()
	if err != nil {
		return "", err
	}
	tok, err := jwt.ParseSigned(signed, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		return "", err
	}
	var gotStd jwt.Claims
	var gotPriv Claims
	if err := tok.Claims(&s.ec.PublicKey, &gotStd, &gotPriv); err != nil {
		return "", err
	}
	verr := gotStd.ValidateWithLeeway(jwt.Expected{Issuer: "traefik", AnyAudience: jwt.Audience{"plugin"}, Time: now}, 0)

	nested, err := jwt.SignedAndEncrypted(s.signer, s.enc).Claims(priv).Serialize()
	if err != nil {
		return "", err
	}
	nt, err := jwt.ParseSignedAndEncrypted(nested, []jose.KeyAlgorithm{jose.RSA_OAEP_256}, []jose.ContentEncryption{jose.A256GCM},
		[]jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		return "", err
	}
	inner, err := nt.Decrypt(s.rsa)
	if err != nil {
		return "", err
	}
	var nestedClaims Claims
	if err := inner.Claims(&s.ec.PublicKey, &nestedClaims); err != nil {
		return "", err
	}
	// A token signed by another key must be refused.
	other := fixedEC()
	other.D = new(big.Int).Add(other.D, big.NewInt(1))
	other.X, other.Y = other.Curve.ScalarBaseMult(other.D.Bytes())
	badErr := tok.Claims(&other.PublicKey, &gotStd)

	return fmt.Sprintf("sub=%s path=%s roles=%v valid=%v nested=%s/%s kid=%s bad=%t jwks=%d",
		gotStd.Subject, gotPriv.Path, gotPriv.Roles, verr == nil, nestedClaims.Method, nestedClaims.Path,
		tok.Headers[0].KeyID, badErr != nil, s.jwksLen), nil
}
