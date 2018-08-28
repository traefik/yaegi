package stdlib

import "reflect"

var Value = map[string]map[string]reflect.Value{}
var Type = map[string]map[string]reflect.Type{}

// Provide access to go standard library (http://golang.org/pkg/)

//go:generate go run ../cmd/goexports/goexports.go archive/tar archive/zip
//go:generate go run ../cmd/goexports/goexports.go bufio bytes
//go:generate go run ../cmd/goexports/goexports.go compress/bzip2 compress/flate compress/gzip compress/lzw compress/zlib
//go:generate go run ../cmd/goexports/goexports.go container/heap container/list container/ring
//go:generate go run ../cmd/goexports/goexports.go context crypto crypto/aes crypto/cipher crypto/des crypto/dsa
//go:generate go run ../cmd/goexports/goexports.go crypto/ecdsa crypto/elliptic crypto/hmac crypto/md5 crypto/rand
//go:generate go run ../cmd/goexports/goexports.go crypto/rc4 crypto/rsa crypto/sha1 crypto/sha256 crypto/sha512
//go:generate go run ../cmd/goexports/goexports.go crypto/subtle crypto/tls crypto/x509 crypto/x509/pkix
//go:generate go run ../cmd/goexports/goexports.go database/sql database/sql/driver
//go:generate go run ../cmd/goexports/goexports.go encoding encoding/ascii85 encoding/asn1 encoding/base32
//go:generate go run ../cmd/goexports/goexports.go encoding/base64 encoding/binary encoding/csv encoding/gob
//go:generate go run ../cmd/goexports/goexports.go encoding/hex encoding/json encoding/pem encoding/xml
//go:generate go run ../cmd/goexports/goexports.go errors expvar flag fmt
//go:generate go run ../cmd/goexports/goexports.go go/ast go/build go/constant go/doc go/format go/importer
//go:generate go run ../cmd/goexports/goexports.go go/parser go/printer go/scanner go/token go/types
//go:generate go run ../cmd/goexports/goexports.go hash hash/adler32 hash/crc32 hash/crc64 hash/fnv
//go:generate go run ../cmd/goexports/goexports.go html html/template
//go:generate go run ../cmd/goexports/goexports.go image image/color image/color/palette
//go:generate go run ../cmd/goexports/goexports.go image/draw image/gif image/jpeg image/png
//go:generate go run ../cmd/goexports/goexports.go index/suffixarray io io/ioutil log log/syslog
//go:generate go run ../cmd/goexports/goexports.go math math/big math/bits math/cmplx math/rand
//go:generate go run ../cmd/goexports/goexports.go mime mime/multipart mime/quotedprintable
//go:generate go run ../cmd/goexports/goexports.go net net/http net/http/cgi net/http/cookiejar net/http/fcgi
//go:generate go run ../cmd/goexports/goexports.go net/http/httptest net/http/httptrace net/http/httputil
//go:generate go run ../cmd/goexports/goexports.go net/mail net/rpc net/rpc/jsonrpc net/smtp net/textproto net/url
//go:generate go run ../cmd/goexports/goexports.go os os/exec os/signal os/user
//go:generate go run ../cmd/goexports/goexports.go path path/filepath reflect regexp regexp/syntax
//go:generate go run ../cmd/goexports/goexports.go runtime runtime/debug
//go:generate go run ../cmd/goexports/goexports.go sort strconv strings sync sync/atomic
//go:generate go run ../cmd/goexports/goexports.go text/scanner text/tabwriter text/template text/template/parse
//go:generate go run ../cmd/goexports/goexports.go time unsafe
//go:generate go run ../cmd/goexports/goexports.go unicode unicode/utf16 unicode/utf8
