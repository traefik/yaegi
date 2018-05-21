package export

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

// Pkg contains go stdlib packages
var Pkg = &map[string]*map[string]interface{}{
	"archive/tar":          ArchiveTar,
	"archive/zip":          ArchiveZip,
	"bufio":                Bufio,
	"bytes":                Bytes,
	"compress/bzip2":       CompressBzip2,
	"compress/flate":       CompressFlate,
	"compress/gzip":        CompressGzip,
	"compress/lzw":         CompressLzw,
	"compress/zlib":        CompressZlib,
	"container/heap":       ContainerHeap,
	"container/list":       ContainerList,
	"container/ring":       ContainerRing,
	"context":              Context,
	"crypto":               Crypto,
	"crypto/aes":           CryptoAes,
	"crypto/cipher":        CryptoCipher,
	"crypto/des":           CryptoDes,
	"crypto/dsa":           CryptoDsa,
	"crypto/ecdsa":         CryptoEcdsa,
	"crypto/elliptic":      CryptoElliptic,
	"crypto/hmac":          CryptoHmac,
	"crypto/md5":           CryptoMd5,
	"crypto/rand":          CryptoRand,
	"crypto/rc4":           CryptoRc4,
	"crypto/rsa":           CryptoRsa,
	"crypto/sha1":          CryptoSha1,
	"crypto/sha256":        CryptoSha256,
	"crypto/sha512":        CryptoSha512,
	"crypto/subtle":        CryptoSubtle,
	"crypto/tls":           CryptoTLS,
	"crypto/x509":          CryptoX509,
	"crypto/x509/pkix":     X509Pkix,
	"database/sql":         DatabaseSQL,
	"database/sql/driver":  SQLDriver,
	"encoding":             Encoding,
	"encoding/ascii85":     EncodingASCII85,
	"encoding/asn1":        EncodingAsn1,
	"encoding/base32":      EncodingBase32,
	"encoding/base64":      EncodingBase64,
	"encoding/binary":      EncodingBinary,
	"encoding/csv":         EncodingCsv,
	"encoding/gob":         EncodingGob,
	"encoding/hex":         EncodingHex,
	"encoding/json":        EncodingJSON,
	"encoding/pem":         EncodingPem,
	"encoding/xml":         EncodingXML,
	"errors":               Errors,
	"expvar":               Expvar,
	"flag":                 Flag,
	"fmt":                  Fmt,
	"go/ast":               GoAst,
	"go/build":             GoBuild,
	"go/constant":          GoConstant,
	"go/doc":               GoDoc,
	"go/format":            GoFormat,
	"go/importer":          GoImporter,
	"go/parser":            GoParser,
	"go/printer":           GoPrinter,
	"go/scanner":           GoScanner,
	"go/token":             GoToken,
	"go/types":             GoTypes,
	"hash":                 Hash,
	"hash/adler32":         HashAdler32,
	"hash/crc32":           HashCrc32,
	"hash/crc64":           HashCrc64,
	"hash/fnv":             HashFnv,
	"html":                 HTML,
	"html/template":        HTMLTemplate,
	"image":                Image,
	"image/color":          ImageColor,
	"image/color/palette":  ColorPalette,
	"image/draw":           ImageDraw,
	"image/gif":            ImageGif,
	"image/jpeg":           ImageJpeg,
	"image/png":            ImagePng,
	"index/suffixarray":    IndexSuffixarray,
	"io":                   Io,
	"io/ioutil":            IoIoutil,
	"log":                  Log,
	"log/syslog":           LogSyslog,
	"math":                 Math,
	"math/big":             MathBig,
	"math/bits":            MathBits,
	"math/cmplx":           MathCmplx,
	"math/rand":            MathRand,
	"mime":                 Mime,
	"mime/multipart":       MimeMultipart,
	"mime/quotedprintable": MimeQuotedprintable,
	"net":                 Net,
	"net/http":            NetHTTP,
	"net/http/cgi":        HTTPCgi,
	"net/http/cookiejar":  HTTPCookiejar,
	"net/http/fcgi":       HTTPFcgi,
	"net/http/httptest":   HTTPHttptest,
	"net/http/httptrace":  HTTPHttptrace,
	"net/http/httputil":   HTTPHttputil,
	"net/mail":            NetMail,
	"net/rpc":             NetRPC,
	"net/rpc/jsonrpc":     RPCJsonrpc,
	"net/smtp":            NetSMTP,
	"net/textproto":       NetTextproto,
	"net/url":             NetURL,
	"os":                  Os,
	"os/exec":             OsExec,
	"os/signal":           OsSignal,
	"os/user":             OsUser,
	"path":                Path,
	"path/filepath":       PathFilepath,
	"reflect":             Reflect,
	"regexp":              Regexp,
	"regexp/syntax":       RegexpSyntax,
	"runtime":             Runtime,
	"runtime/debug":       RuntimeDebug,
	"sort":                Sort,
	"strconv":             Strconv,
	"strings":             Strings,
	"sync":                Sync,
	"sync/atomic":         SyncAtomic,
	"text/scanner":        TextScanner,
	"text/tabwriter":      TextTabwriter,
	"text/template":       TextTemplate,
	"text/template/parse": TemplateParse,
	"time":                Time,
	"unicode":             Unicode,
	"unicode/utf16":       UnicodeUtf16,
	"unicode/utf8":        UnicodeUtf8,
	"unsafe":              Unsafe,
}
