// Code generated by 'goexports crypto/tls'. DO NOT EDIT.

// +build go1.14,!go1.15

package stdlib

import (
	"crypto/tls"
	"reflect"
)

func init() {
	Symbols["crypto/tls"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"CipherSuiteName":                         reflect.ValueOf(tls.CipherSuiteName),
		"CipherSuites":                            reflect.ValueOf(tls.CipherSuites),
		"Client":                                  reflect.ValueOf(tls.Client),
		"CurveP256":                               reflect.ValueOf(tls.CurveP256),
		"CurveP384":                               reflect.ValueOf(tls.CurveP384),
		"CurveP521":                               reflect.ValueOf(tls.CurveP521),
		"Dial":                                    reflect.ValueOf(tls.Dial),
		"DialWithDialer":                          reflect.ValueOf(tls.DialWithDialer),
		"ECDSAWithP256AndSHA256":                  reflect.ValueOf(tls.ECDSAWithP256AndSHA256),
		"ECDSAWithP384AndSHA384":                  reflect.ValueOf(tls.ECDSAWithP384AndSHA384),
		"ECDSAWithP521AndSHA512":                  reflect.ValueOf(tls.ECDSAWithP521AndSHA512),
		"ECDSAWithSHA1":                           reflect.ValueOf(tls.ECDSAWithSHA1),
		"Ed25519":                                 reflect.ValueOf(tls.Ed25519),
		"InsecureCipherSuites":                    reflect.ValueOf(tls.InsecureCipherSuites),
		"Listen":                                  reflect.ValueOf(tls.Listen),
		"LoadX509KeyPair":                         reflect.ValueOf(tls.LoadX509KeyPair),
		"NewLRUClientSessionCache":                reflect.ValueOf(tls.NewLRUClientSessionCache),
		"NewListener":                             reflect.ValueOf(tls.NewListener),
		"NoClientCert":                            reflect.ValueOf(tls.NoClientCert),
		"PKCS1WithSHA1":                           reflect.ValueOf(tls.PKCS1WithSHA1),
		"PKCS1WithSHA256":                         reflect.ValueOf(tls.PKCS1WithSHA256),
		"PKCS1WithSHA384":                         reflect.ValueOf(tls.PKCS1WithSHA384),
		"PKCS1WithSHA512":                         reflect.ValueOf(tls.PKCS1WithSHA512),
		"PSSWithSHA256":                           reflect.ValueOf(tls.PSSWithSHA256),
		"PSSWithSHA384":                           reflect.ValueOf(tls.PSSWithSHA384),
		"PSSWithSHA512":                           reflect.ValueOf(tls.PSSWithSHA512),
		"RenegotiateFreelyAsClient":               reflect.ValueOf(tls.RenegotiateFreelyAsClient),
		"RenegotiateNever":                        reflect.ValueOf(tls.RenegotiateNever),
		"RenegotiateOnceAsClient":                 reflect.ValueOf(tls.RenegotiateOnceAsClient),
		"RequestClientCert":                       reflect.ValueOf(tls.RequestClientCert),
		"RequireAndVerifyClientCert":              reflect.ValueOf(tls.RequireAndVerifyClientCert),
		"RequireAnyClientCert":                    reflect.ValueOf(tls.RequireAnyClientCert),
		"Server":                                  reflect.ValueOf(tls.Server),
		"TLS_AES_128_GCM_SHA256":                  reflect.ValueOf(tls.TLS_AES_128_GCM_SHA256),
		"TLS_AES_256_GCM_SHA384":                  reflect.ValueOf(tls.TLS_AES_256_GCM_SHA384),
		"TLS_CHACHA20_POLY1305_SHA256":            reflect.ValueOf(tls.TLS_CHACHA20_POLY1305_SHA256),
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA),
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256": reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256),
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256),
		"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA),
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384),
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305),
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256),
		"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":              reflect.ValueOf(tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA),
		"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":           reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA),
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":            reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA),
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":         reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256),
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256),
		"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":            reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA),
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384),
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":          reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305),
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256),
		"TLS_ECDHE_RSA_WITH_RC4_128_SHA":                reflect.ValueOf(tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA),
		"TLS_FALLBACK_SCSV":                             reflect.ValueOf(tls.TLS_FALLBACK_SCSV),
		"TLS_RSA_WITH_3DES_EDE_CBC_SHA":                 reflect.ValueOf(tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA),
		"TLS_RSA_WITH_AES_128_CBC_SHA":                  reflect.ValueOf(tls.TLS_RSA_WITH_AES_128_CBC_SHA),
		"TLS_RSA_WITH_AES_128_CBC_SHA256":               reflect.ValueOf(tls.TLS_RSA_WITH_AES_128_CBC_SHA256),
		"TLS_RSA_WITH_AES_128_GCM_SHA256":               reflect.ValueOf(tls.TLS_RSA_WITH_AES_128_GCM_SHA256),
		"TLS_RSA_WITH_AES_256_CBC_SHA":                  reflect.ValueOf(tls.TLS_RSA_WITH_AES_256_CBC_SHA),
		"TLS_RSA_WITH_AES_256_GCM_SHA384":               reflect.ValueOf(tls.TLS_RSA_WITH_AES_256_GCM_SHA384),
		"TLS_RSA_WITH_RC4_128_SHA":                      reflect.ValueOf(tls.TLS_RSA_WITH_RC4_128_SHA),
		"VerifyClientCertIfGiven":                       reflect.ValueOf(tls.VerifyClientCertIfGiven),
		"VersionSSL30":                                  reflect.ValueOf(tls.VersionSSL30),
		"VersionTLS10":                                  reflect.ValueOf(tls.VersionTLS10),
		"VersionTLS11":                                  reflect.ValueOf(tls.VersionTLS11),
		"VersionTLS12":                                  reflect.ValueOf(tls.VersionTLS12),
		"VersionTLS13":                                  reflect.ValueOf(tls.VersionTLS13),
		"X25519":                                        reflect.ValueOf(tls.X25519),
		"X509KeyPair":                                   reflect.ValueOf(tls.X509KeyPair),

		// type definitions
		"Certificate":            reflect.ValueOf((*tls.Certificate)(nil)),
		"CertificateRequestInfo": reflect.ValueOf((*tls.CertificateRequestInfo)(nil)),
		"CipherSuite":            reflect.ValueOf((*tls.CipherSuite)(nil)),
		"ClientAuthType":         reflect.ValueOf((*tls.ClientAuthType)(nil)),
		"ClientHelloInfo":        reflect.ValueOf((*tls.ClientHelloInfo)(nil)),
		"ClientSessionCache":     reflect.ValueOf((*tls.ClientSessionCache)(nil)),
		"ClientSessionState":     reflect.ValueOf((*tls.ClientSessionState)(nil)),
		"Config":                 reflect.ValueOf((*tls.Config)(nil)),
		"Conn":                   reflect.ValueOf((*tls.Conn)(nil)),
		"ConnectionState":        reflect.ValueOf((*tls.ConnectionState)(nil)),
		"CurveID":                reflect.ValueOf((*tls.CurveID)(nil)),
		"RecordHeaderError":      reflect.ValueOf((*tls.RecordHeaderError)(nil)),
		"RenegotiationSupport":   reflect.ValueOf((*tls.RenegotiationSupport)(nil)),
		"SignatureScheme":        reflect.ValueOf((*tls.SignatureScheme)(nil)),

		// interface wrapper definitions
		"_ClientSessionCache": reflect.ValueOf((*_crypto_tls_ClientSessionCache)(nil)),
	}
}

// _crypto_tls_ClientSessionCache is an interface wrapper for ClientSessionCache type
type _crypto_tls_ClientSessionCache struct {
	WGet func(sessionKey string) (session *tls.ClientSessionState, ok bool)
	WPut func(sessionKey string, cs *tls.ClientSessionState)
}

func (W _crypto_tls_ClientSessionCache) Get(sessionKey string) (session *tls.ClientSessionState, ok bool) {
	return W.WGet(sessionKey)
}
func (W _crypto_tls_ClientSessionCache) Put(sessionKey string, cs *tls.ClientSessionState) {
	W.WPut(sessionKey, cs)
}
