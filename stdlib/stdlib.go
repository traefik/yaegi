// +build go1.13,!go1.15

// Package stdlib provides wrappers of standard library packages to be imported natively in Yaegi.
package stdlib

import "reflect"

// Symbols variable stores the map of stdlib symbols per package.
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/containous/yaegi/stdlib"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

// Provide access to go standard library (http://golang.org/pkg/)
// go list std | grep -v internal | grep -v '\.' | grep -v unsafe | grep -v syscall

//go:generate ../cmd/goexports/goexports archive/tar archive/zip
//go:generate ../cmd/goexports/goexports bufio bytes
//go:generate ../cmd/goexports/goexports compress/bzip2 compress/flate compress/gzip compress/lzw compress/zlib
//go:generate ../cmd/goexports/goexports container/heap container/list container/ring
//go:generate ../cmd/goexports/goexports context crypto crypto/aes crypto/cipher crypto/des crypto/dsa crypto/ecdsa
//go:generate ../cmd/goexports/goexports crypto/ed25519 crypto/elliptic crypto/hmac crypto/md5 crypto/rand
//go:generate ../cmd/goexports/goexports crypto/rc4 crypto/rsa crypto/sha1 crypto/sha256 crypto/sha512
//go:generate ../cmd/goexports/goexports crypto/subtle crypto/tls crypto/x509 crypto/x509/pkix
//go:generate ../cmd/goexports/goexports database/sql database/sql/driver
//go:generate ../cmd/goexports/goexports debug/dwarf debug/elf debug/gosym debug/macho debug/pe debug/plan9obj
//go:generate ../cmd/goexports/goexports encoding encoding/ascii85 encoding/asn1 encoding/base32
//go:generate ../cmd/goexports/goexports encoding/base64 encoding/binary encoding/csv encoding/gob
//go:generate ../cmd/goexports/goexports encoding/hex encoding/json encoding/pem encoding/xml
//go:generate ../cmd/goexports/goexports errors expvar flag fmt
//go:generate ../cmd/goexports/goexports go/ast go/build go/constant go/doc go/format go/importer
//go:generate ../cmd/goexports/goexports go/parser go/printer go/scanner go/token go/types
//go:generate ../cmd/goexports/goexports hash hash/adler32 hash/crc32 hash/crc64 hash/fnv
//go:generate ../cmd/goexports/goexports html html/template
//go:generate ../cmd/goexports/goexports image image/color image/color/palette
//go:generate ../cmd/goexports/goexports image/draw image/gif image/jpeg image/png index/suffixarray
//go:generate ../cmd/goexports/goexports io io/ioutil log log/syslog
//go:generate ../cmd/goexports/goexports math math/big math/bits math/cmplx math/rand
//go:generate ../cmd/goexports/goexports mime mime/multipart mime/quotedprintable
//go:generate ../cmd/goexports/goexports net net/http net/http/cgi net/http/cookiejar net/http/fcgi
//go:generate ../cmd/goexports/goexports net/http/httptest net/http/httptrace net/http/httputil net/http/pprof
//go:generate ../cmd/goexports/goexports net/mail net/rpc net/rpc/jsonrpc net/smtp net/textproto net/url
//go:generate ../cmd/goexports/goexports os os/signal os/user
//go:generate ../cmd/goexports/goexports path path/filepath reflect regexp regexp/syntax
//go:generate ../cmd/goexports/goexports runtime runtime/debug runtime/pprof runtime/trace
//go:generate ../cmd/goexports/goexports sort strconv strings sync sync/atomic
//go:generate ../cmd/goexports/goexports text/scanner text/tabwriter text/template text/template/parse
//go:generate ../cmd/goexports/goexports time unicode unicode/utf16 unicode/utf8
