// +build go1.14,!go1.16

// Package stdlib provides wrappers of standard library packages to be imported natively in Yaegi.
package stdlib

import "reflect"

// Symbols variable stores the map of stdlib symbols per package.
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/traefik/yaegi/stdlib"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

// Provide access to go standard library (http://golang.org/pkg/)
// go list std | grep -v internal | grep -v '\.' | grep -v unsafe | grep -v syscall

//go:generate ../internal/extract/extract archive/tar archive/zip
//go:generate ../internal/extract/extract bufio bytes
//go:generate ../internal/extract/extract compress/bzip2 compress/flate compress/gzip compress/lzw compress/zlib
//go:generate ../internal/extract/extract container/heap container/list container/ring
//go:generate ../internal/extract/extract context crypto crypto/aes crypto/cipher crypto/des crypto/dsa crypto/ecdsa
//go:generate ../internal/extract/extract crypto/ed25519 crypto/elliptic crypto/hmac crypto/md5 crypto/rand
//go:generate ../internal/extract/extract crypto/rc4 crypto/rsa crypto/sha1 crypto/sha256 crypto/sha512
//go:generate ../internal/extract/extract crypto/subtle crypto/tls crypto/x509 crypto/x509/pkix
//go:generate ../internal/extract/extract database/sql database/sql/driver
//go:generate ../internal/extract/extract debug/dwarf debug/elf debug/gosym debug/macho debug/pe debug/plan9obj
//go:generate ../internal/extract/extract encoding encoding/ascii85 encoding/asn1 encoding/base32
//go:generate ../internal/extract/extract encoding/base64 encoding/binary encoding/csv encoding/gob
//go:generate ../internal/extract/extract encoding/hex encoding/json encoding/pem encoding/xml
//go:generate ../internal/extract/extract errors expvar flag fmt
//go:generate ../internal/extract/extract go/ast go/build go/constant go/doc go/format go/importer
//go:generate ../internal/extract/extract go/parser go/printer go/scanner go/token go/types
//go:generate ../internal/extract/extract hash hash/adler32 hash/crc32 hash/crc64 hash/fnv hash/maphash
//go:generate ../internal/extract/extract html html/template
//go:generate ../internal/extract/extract image image/color image/color/palette
//go:generate ../internal/extract/extract image/draw image/gif image/jpeg image/png index/suffixarray
//go:generate ../internal/extract/extract io io/ioutil log log/syslog
//go:generate ../internal/extract/extract math math/big math/bits math/cmplx math/rand
//go:generate ../internal/extract/extract mime mime/multipart mime/quotedprintable
//go:generate ../internal/extract/extract net net/http net/http/cgi net/http/cookiejar net/http/fcgi
//go:generate ../internal/extract/extract net/http/httptest net/http/httptrace net/http/httputil net/http/pprof
//go:generate ../internal/extract/extract net/mail net/rpc net/rpc/jsonrpc net/smtp net/textproto net/url
//go:generate ../internal/extract/extract os os/signal os/user
//go:generate ../internal/extract/extract path path/filepath reflect regexp regexp/syntax
//go:generate ../internal/extract/extract runtime runtime/debug runtime/pprof runtime/trace
//go:generate ../internal/extract/extract sort strconv strings sync sync/atomic
//go:generate ../internal/extract/extract testing testing/iotest testing/quick
//go:generate ../internal/extract/extract text/scanner text/tabwriter text/template text/template/parse
//go:generate ../internal/extract/extract time unicode unicode/utf16 unicode/utf8
