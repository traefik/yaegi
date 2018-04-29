package export

// Provide access to go standard library (http://golang.org/pkg/)

//go:generate go run gen.go archive/tar archive/zip
//go:generate go run gen.go bufio bytes
//go:generate go run gen.go compress/bzip2 compress/flate compress/gzip compress/lzw compress/zlib
//go:generate go run gen.go container/heap container/list container/ring
//go:generate go run gen.go context crypto crypto/aes crypto/cipher crypto/des crypto/dsa
//go:generate go run gen.go crypto/ecdsa crypto/elliptic crypto/hmac crypto/md5 crypto/rand
//go:generate go run gen.go crypto/rc4 crypto/rsa crypto/sha1 crypto/sha256 crypto/sha512
//go:generate go run gen.go crypto/subtle crypto/tls crypto/x509 crypto/x509/pkix
//go:generate go run gen.go database/sql database/sql/driver
//go:generate go run gen.go encoding encoding/ascii85 encoding/asn1 encoding/base32
//go:generate go run gen.go encoding/base64 encoding/binary encoding/csv encoding/gob
//go:generate go run gen.go encoding/hex encoding/json encoding/pem encoding/xml
//go:generate go run gen.go errors expvar flag fmt
//go:generate go run gen.go go/ast go/build go/constant go/doc go/format go/importer
//go:generate go run gen.go go/parser go/printer go/scanner go/token go/types
//go:generate go run gen.go hash hash/adler32 hash/crc32 hash/crc64 hash/fnv
//go:generate go run gen.go html html/template
//go:generate go run gen.go image image/color image/color/palette
//go:generate go run gen.go image/draw image/gif image/jpeg image/png
//go:generate go run gen.go index/suffixarray io io/ioutil log log/syslog
//go:generate go run gen.go math math/big math/bits math/cmplx math/rand
//go:generate go run gen.go mime mime/multipart mime/quotedprintable
//go:generate go run gen.go net net/http net/http/cgi net/http/cookiejar net/http/fcgi
//go:generate go run gen.go net/http/httptest net/http/httptrace net/http/httputil
//go:generate go run gen.go net/mail net/rpc net/rpc/jsonrpc net/smtp net/textproto net/url
//go:generate go run gen.go os os/exec os/signal os/user
//go:generate go run gen.go path path/filepath reflect regexp regexp/syntax
//go:generate go run gen.go runtime runtime/debug
//go:generate go run gen.go sort strconv strings sync sync/atomic
//go:generate go run gen.go text/scanner text/tabwriter text/template text/template/parse
//go:generate go run gen.go time unsafe
//go:generate go run gen.go unicode unicode/utf16 unicode/utf8

var Pkg = &map[string]*map[string]interface{}{
	"archive/tar":          sym_tar,
	"archive/zip":          sym_zip,
	"bufio":                sym_bufio,
	"bytes":                sym_bytes,
	"compress/bzip2":       sym_bzip2,
	"compress/flate":       sym_flate,
	"compress/gzip":        sym_gzip,
	"compress/lzw":         sym_lzw,
	"compress/zlib":        sym_zlib,
	"container/heap":       sym_heap,
	"container/list":       sym_list,
	"container/ring":       sym_ring,
	"context":              sym_context,
	"crypto":               sym_crypto,
	"crypto/aes":           sym_aes,
	"crypto/cipher":        sym_cipher,
	"crypto/des":           sym_des,
	"crypto/dsa":           sym_dsa,
	"crypto/ecdsa":         sym_ecdsa,
	"crypto/elliptic":      sym_elliptic,
	"crypto/hmac":          sym_hmac,
	"crypto/md5":           sym_md5,
	"crypto/rand":          sym_rand,
	"crypto/rc4":           sym_rc4,
	"crypto/rsa":           sym_rsa,
	"crypto/sha1":          sym_sha1,
	"crypto/sha256":        sym_sha256,
	"crypto/sha512":        sym_sha512,
	"crypto/subtle":        sym_subtle,
	"crypto/tls":           sym_tls,
	"crypto/x509":          sym_x509,
	"crypto/x509/pkix":     sym_pkix,
	"database/sql":         sym_sql,
	"database/sql/driver":  sym_driver,
	"encoding":             sym_encoding,
	"encoding/ascii85":     sym_ascii85,
	"encoding/asn1":        sym_asn1,
	"encoding/base32":      sym_base32,
	"encoding/base64":      sym_base64,
	"encoding/binary":      sym_binary,
	"encoding/csv":         sym_csv,
	"encoding/gob":         sym_gob,
	"encoding/hex":         sym_hex,
	"encoding/json":        sym_json,
	"encoding/pem":         sym_pem,
	"encoding/xml":         sym_xml,
	"errors":               sym_errors,
	"expvar":               sym_expvar,
	"flag":                 sym_flag,
	"fmt":                  sym_fmt,
	"go/ast":               sym_ast,
	"go/build":             sym_build,
	"go/constant":          sym_constant,
	"go/doc":               sym_doc,
	"go/format":            sym_format,
	"go/importer":          sym_importer,
	"go/parser":            sym_parser,
	"go/printer":           sym_printer,
	"go/scanner":           sym_scanner,
	"go/token":             sym_token,
	"go/types":             sym_types,
	"hash":                 sym_hash,
	"hash/adler32":         sym_adler32,
	"hash/crc32":           sym_crc32,
	"hash/crc64":           sym_crc64,
	"hash/fnv":             sym_fnv,
	"html":                 sym_html,
	"html/template":        sym_template,
	"image":                sym_image,
	"image/color":          sym_color,
	"image/color/palette":  sym_palette,
	"image/draw":           sym_draw,
	"image/gif":            sym_gif,
	"image/jpeg":           sym_jpeg,
	"image/png":            sym_png,
	"index/suffixarray":    sym_suffixarray,
	"io":                   sym_io,
	"io/ioutil":            sym_ioutil,
	"log":                  sym_log,
	"log/syslog":           sym_syslog,
	"math":                 sym_math,
	"math/big":             sym_big,
	"math/bits":            sym_bits,
	"math/cmplx":           sym_cmplx,
	"math/rand":            sym_math_rand,
	"mime":                 sym_mime,
	"mime/multipart":       sym_multipart,
	"mime/quotedprintable": sym_quotedprintable,
	"net":                 sym_net,
	"net/http":            sym_http,
	"net/http/cgi":        sym_cgi,
	"net/http/cookiejar":  sym_cookiejar,
	"net/http/fcgi":       sym_fcgi,
	"net/http/httptest":   sym_httptest,
	"net/http/httptrace":  sym_httptrace,
	"net/http/httputil":   sym_httputil,
	"net/mail":            sym_mail,
	"net/rpc":             sym_rpc,
	"net/rpc/jsonrpc":     sym_jsonrpc,
	"net/smtp":            sym_smtp,
	"net/textproto":       sym_textproto,
	"net/url":             sym_url,
	"os":                  sym_os,
	"os/exec":             sym_exec,
	"os/signal":           sym_signal,
	"os/user":             sym_user,
	"path":                sym_path,
	"path/filepath":       sym_filepath,
	"reflect":             sym_reflect,
	"regexp":              sym_regexp,
	"regexp/syntax":       sym_syntax,
	"runtime":             sym_runtime,
	"runtime/debug":       sym_debug,
	"sort":                sym_sort,
	"strconv":             sym_strconv,
	"strings":             sym_strings,
	"sync":                sym_sync,
	"sync/atomic":         sym_atomic,
	"text/scanner":        sym_text_scanner,
	"text/tabwriter":      sym_tabwriter,
	"text/template":       sym_template,
	"text/template/parse": sym_parse,
	"time":                sym_time,
	"unicode":             sym_unicode,
	"unicode/utf16":       sym_utf16,
	"unicode/utf8":        sym_utf8,
	"unsafe":              sym_unsafe,
}
