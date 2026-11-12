//go:build ignore

// optcombos_gen writes zz_optcombos.go: for each family of optwrap.go, one
// wrapper type per combination of its optional methods. Run it with
// "go generate ./vm".
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"log"
	"os"
	"strings"
)

// method is a method of a family: its parameter and result types, as Go
// code of zz_optcombos.go.
type method struct {
	name            string
	params, results []string
}

func m(name string, params, results string) method {
	sp := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ", ")
	}
	return method{name, sp(params), sp(results)}
}

// family is a set of wrappers: the base methods, that every value of the
// family has, and the optional methods that the standard library looks
// for with a type assertion (docs/BOUNDARY.md). obj families embed vmObj
// in their base, as the hand-written wrappers do (Format, MarshalJSON).
// Two optional methods with the same name (Unwrap() error and Unwrap()
// []error) never go together.
type family struct {
	name  string
	obj   bool
	base  []method
	feats []method
}

var families = []family{
	{name: "Reader", base: []method{m("Read", "[]byte", "int, error")}, feats: []method{
		m("WriteTo", "io.Writer", "int64, error"),
		m("ReadByte", "", "byte, error"),
		m("UnreadByte", "", "error"),
		m("ReadRune", "", "rune, int, error"),
		m("UnreadRune", "", "error"),
		m("Close", "", "error"),
		m("Seek", "int64, int", "int64, error"),
	}},
	{name: "ReadWriter", base: []method{m("Read", "[]byte", "int, error"), m("Write", "[]byte", "int, error")}, feats: []method{
		m("WriteTo", "io.Writer", "int64, error"),
		m("ReadFrom", "io.Reader", "int64, error"),
		m("Close", "", "error"),
		m("Seek", "int64, int", "int64, error"),
		// the body of a 101 response (httputil.ReverseProxy, Transport)
		m("CloseWrite", "", "error"),
	}},
	{name: "Writer", base: []method{m("Write", "[]byte", "int, error")}, feats: []method{
		m("ReadFrom", "io.Reader", "int64, error"),
		m("WriteString", "string", "int, error"),
		m("WriteByte", "byte", "error"),
		m("Close", "", "error"),
		m("Flush", "", "error"),
	}},
	{name: "Error", obj: true, base: []method{m("Error", "", "string")}, feats: []method{
		m("Unwrap", "", "error"),
		m("Unwrap", "", "[]error"),
		m("Is", "error", "bool"),
		m("As", "any", "bool"),
		m("Timeout", "", "bool"),
		m("Temporary", "", "bool"),
		m("String", "", "string"),
		m("LogValue", "", "slog.Value"),
	}},
	// log/slog calls LogValue of the values it logs (slog.LogValuer): a
	// type that hides a secret in its logs. Without the method, native slog
	// printed the fields of the plugin value (optwrap.go, logValue).
	{name: "LogValuer", obj: true, base: []method{m("LogValue", "", "slog.Value")}, feats: []method{
		m("String", "", "string"),
	}},
	{name: "FlagValue", base: []method{m("String", "", "string"), m("Set", "string", "error")}, feats: []method{
		m("IsBoolFlag", "", "bool"),
		m("Get", "", "any"),
	}},
	{name: "Source", base: []method{m("Int63", "", "int64"), m("Seed", "int64", "")}, feats: []method{
		m("Uint64", "", "uint64"),
	}},
	{name: "FS", base: []method{m("Open", "string", "fs.File, error")}, feats: []method{
		m("ReadDir", "string", "[]fs.DirEntry, error"),
		m("ReadFile", "string", "[]byte, error"),
		m("Stat", "string", "fs.FileInfo, error"),
		m("Glob", "string", "[]string, error"),
		m("Sub", "string", "fs.FS, error"),
		m("ReadLink", "string", "string, error"),
		m("Lstat", "string", "fs.FileInfo, error"),
	}},
	{name: "File", base: []method{m("Stat", "", "fs.FileInfo, error"), m("Read", "[]byte", "int, error"), m("Close", "", "error")}, feats: []method{
		m("ReadDir", "int", "[]fs.DirEntry, error"),
		m("Readdir", "int", "[]fs.FileInfo, error"),
		m("Seek", "int64, int", "int64, error"),
		m("ReadAt", "[]byte, int64", "int, error"),
	}},
	{name: "Context", base: []method{m("Deadline", "", "time.Time, bool"), m("Done", "", "<-chan struct{}"), m("Err", "", "error"), m("Value", "any", "any")}, feats: []method{
		m("AfterFunc", "func()", "func() bool"),
	}},
	{name: "Conn", base: []method{
		m("Read", "[]byte", "int, error"), m("Write", "[]byte", "int, error"), m("Close", "", "error"),
		m("LocalAddr", "", "net.Addr"), m("RemoteAddr", "", "net.Addr"),
		m("SetDeadline", "time.Time", "error"), m("SetReadDeadline", "time.Time", "error"), m("SetWriteDeadline", "time.Time", "error"),
	}, feats: []method{
		m("ReadFrom", "[]byte", "int, net.Addr, error"),
		m("WriteTo", "[]byte, net.Addr", "int, error"),
		m("CloseWrite", "", "error"),
		m("ReadFrom", "io.Reader", "int64, error"),
		m("WriteTo", "io.Writer", "int64, error"),
		// net/http asks a conn for the TLS state (connectionStater) and for
		// a handshake (handshaker).
		m("ConnectionState", "", "tls.ConnectionState"),
		m("HandshakeContext", "context.Context", "error"),
	}},
	{name: "Signer", base: []method{m("Public", "", "crypto.PublicKey"), m("Sign", "io.Reader, []byte, crypto.SignerOpts", "[]byte, error")}, feats: []method{
		m("Decrypt", "io.Reader, []byte, crypto.DecrypterOpts", "[]byte, error"),
		m("SignMessage", "io.Reader, []byte, crypto.SignerOpts", "[]byte, error"),
		m("Equal", "crypto.PrivateKey", "bool"),
	}},
	{name: "Decrypter", base: []method{m("Public", "", "crypto.PublicKey"), m("Decrypt", "io.Reader, []byte, crypto.DecrypterOpts", "[]byte, error")}, feats: []method{
		m("Equal", "crypto.PrivateKey", "bool"),
	}},
	{name: "Image", base: []method{m("ColorModel", "", "color.Model"), m("Bounds", "", "image.Rectangle"), m("At", "int, int", "color.Color")}, feats: []method{
		m("Opaque", "", "bool"),
		m("RGBA64At", "int, int", "color.RGBA64"),
		m("ColorIndexAt", "int, int", "uint8"),
		m("Set", "int, int, color.Color", ""),
		m("SetRGBA64", "int, int, color.RGBA64", ""),
	}},
	{name: "RoundTripper", base: []method{m("RoundTrip", "*http.Request", "*http.Response, error")}, feats: []method{
		m("CancelRequest", "*http.Request", ""),
		m("CloseIdleConnections", "", ""),
		// the RoundTripper of TLSNextProto: an error to give back at once,
		// and Transport.NewClientConn
		m("RoundTripErr", "", "error"),
		m("NewClientConn", "net.Conn, func()", "http.RoundTripper, error"),
	}},
	// The client connection that NewClientConn of a RoundTripper gives to
	// Transport.NewClientConn (net/http's genericClientConn). A value of
	// this family is also a RoundTripper, and optFor takes the family with
	// the most methods: the optional methods of the RoundTripper family are
	// here too, or a RoundTripper with the methods of a client connection
	// would lose them.
	{name: "ClientConn", base: []method{
		m("RoundTrip", "*http.Request", "*http.Response, error"), m("Close", "", "error"), m("Err", "", "error"),
		m("Reserve", "", "error"), m("Release", "", ""), m("Available", "", "int"), m("InFlight", "", "int"),
	}, feats: []method{
		m("Ping", "context.Context", "error"),
		m("CancelRequest", "*http.Request", ""),
		m("CloseIdleConnections", "", ""),
		m("RoundTripErr", "", "error"),
		m("NewClientConn", "net.Conn, func()", "http.RoundTripper, error"),
	}},
	// net/http asks the listener of Server.Serve for an HTTP/2
	// configuration (http2ExternalServerConfig, Go 1.27).
	{name: "Listener", base: []method{m("Accept", "", "net.Conn, error"), m("Close", "", "error"), m("Addr", "", "net.Addr")}, feats: []method{
		m("HTTP2Config", "", "http.HTTP2Config"),
		m("IdleTimeout", "", "time.Duration"),
		m("ServeConnFunc", "serveConnFunc", ""),
	}},
	// crypto/cipher asks a Block for its own modes (cbcEncAble, cbcDecAble,
	// ctrAble, gcmAble).
	{name: "Block", base: []method{m("BlockSize", "", "int"), m("Encrypt", "[]byte, []byte", ""), m("Decrypt", "[]byte, []byte", "")}, feats: []method{
		m("NewCBCEncrypter", "[]byte", "cipher.BlockMode"),
		m("NewCBCDecrypter", "[]byte", "cipher.BlockMode"),
		m("NewCTR", "[]byte", "cipher.Stream"),
		m("NewGCM", "int, int", "cipher.AEAD, error"),
	}},
	// crypto/elliptic asks a Curve for its own Unmarshal (unmarshaler).
	{name: "Curve", base: []method{
		m("Params", "", "*elliptic.CurveParams"), m("IsOnCurve", "*big.Int, *big.Int", "bool"),
		m("Add", "*big.Int, *big.Int, *big.Int, *big.Int", "*big.Int, *big.Int"), m("Double", "*big.Int, *big.Int", "*big.Int, *big.Int"),
		m("ScalarMult", "*big.Int, *big.Int, []byte", "*big.Int, *big.Int"), m("ScalarBaseMult", "[]byte", "*big.Int, *big.Int"),
	}, feats: []method{
		m("Unmarshal", "[]byte", "*big.Int, *big.Int"),
		m("UnmarshalCompressed", "[]byte", "*big.Int, *big.Int"),
	}},
	// crypto/hmac saves the state of a hash that can marshal it, and clones
	// a hash.Cloner.
	{name: "Hash", base: []method{
		m("Write", "[]byte", "int, error"), m("Sum", "[]byte", "[]byte"), m("Reset", "", ""), m("Size", "", "int"), m("BlockSize", "", "int"),
	}, feats: []method{
		m("MarshalBinary", "", "[]byte, error"),
		m("UnmarshalBinary", "[]byte", "error"),
		m("Clone", "", "hash.Cloner, error"),
	}},
}

// body writes the method mt of a wrapper whose vmObj is recv.
func body(b *bytes.Buffer, typ, recv string, mt method) {
	var ps, args []string
	for i, p := range mt.params {
		ps = append(ps, fmt.Sprintf("a%d %s", i, p))
		args = append(args, fmt.Sprintf("reflect.ValueOf(&a%d).Elem()", i))
	}
	res := strings.Join(mt.results, ", ")
	if len(mt.results) > 1 {
		res = "(" + res + ")"
	}
	call := fmt.Sprintf("w.%s.call(%q%s)", recv, mt.name, strings.Join(append([]string{""}, args...), ", "))
	fmt.Fprintf(b, "func (w %s) %s(%s) %s {\n", typ, mt.name, strings.Join(ps, ", "), res)
	if mt.name == "LogValue" {
		// the result may be a slog.Value of the AOT copy (optwrap.go)
		fmt.Fprintf(b, "return w.%s.logValue()\n}\n\n", recv)
		return
	}
	// The zero value of a wrapper (reflect.Zero of its type: flag's
	// isZeroValue) has no plugin value: its methods return zero values, as
	// the generic wrappers do.
	if len(mt.results) == 0 {
		fmt.Fprintf(b, "if w.%s.m == nil {\nreturn\n}\n%s\n}\n\n", recv, call)
		return
	}
	var zeros []string
	for i, r := range mt.results {
		fmt.Fprintf(b, "var r%d %s\n", i, r)
		zeros = append(zeros, fmt.Sprintf("r%d", i))
	}
	fmt.Fprintf(b, "if w.%s.m == nil {\nreturn %s\n}\n", recv, strings.Join(zeros, ", "))
	var outs []string
	for i, r := range mt.results {
		outs = append(outs, fmt.Sprintf("rwOut[%s](out, %d)", r, i))
	}
	fmt.Fprintf(b, "out := %s\nreturn %s\n}\n\n", call, strings.Join(outs, ", "))
}

// featType is the name of the struct of optional method i of f.
func featType(f family, i int) string {
	n := f.feats[i].name
	for j := 0; j < i; j++ {
		if f.feats[j].name == n {
			n += fmt.Sprint(i)
		}
	}
	return "opt" + f.name + n
}

func sigType(mt method) string {
	res := strings.Join(mt.results, ", ")
	if len(mt.results) > 1 {
		res = "(" + res + ")"
	}
	return fmt.Sprintf("func(%s) %s", strings.Join(mt.params, ", "), res)
}

func main() {
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by optcombos_gen.go; DO NOT EDIT.\n\npackage vm\n\nimport (\n\"context\"\n\"crypto\"\n\"crypto/cipher\"\n\"crypto/elliptic\"\n\"crypto/tls\"\n\"hash\"\n\"image\"\n\"image/color\"\n\"io\"\n\"io/fs\"\n\"log/slog\"\n\"math/big\"\n\"net\"\n\"net/http\"\n\"reflect\"\n\"time\"\n)\n\n")
	var table bytes.Buffer
	fmt.Fprintf(&table, "// optFamilies are the families of optwrap.go.\nvar optFamilies = []*optFamily{\n")
	for _, f := range families {
		base := "opt" + f.name + "Base"
		recv := "o"
		if f.obj {
			fmt.Fprintf(&out, "type %s struct {\nk any\nvmObj\n}\n\n", base)
			recv = "vmObj"
		} else {
			fmt.Fprintf(&out, "type %s struct{ optBase }\n\n", base)
		}
		for _, mt := range f.base {
			body(&out, base, recv, mt)
		}
		for i, mt := range f.feats {
			ft := featType(f, i)
			fmt.Fprintf(&out, "type %s struct{ o vmObj }\n\n", ft)
			body(&out, ft, "o", mt)
		}
		n := 1 << len(f.feats)
		var types, makers bytes.Buffer
		for mask := 0; mask < n; mask++ {
			var embeds, lits, names []string
			clash := false
			seen := map[string]bool{}
			for i, mt := range f.feats {
				if mask&(1<<i) == 0 {
					continue
				}
				if seen[mt.name] {
					clash = true
				}
				seen[mt.name] = true
				embeds = append(embeds, featType(f, i))
				lits = append(lits, featType(f, i)+"{o}")
				names = append(names, mt.name)
			}
			if clash {
				fmt.Fprintf(&makers, "{},\n")
				continue
			}
			name := fmt.Sprintf("opt%s%d", f.name, mask)
			baseLit := base + "{optBase{k, o}}"
			if f.obj {
				baseLit = base + "{k, o}"
			}
			fmt.Fprintf(&types, "type %s struct { // %s\n%s\n%s\n}\n\n", name, strings.Join(names, ", "), base, strings.Join(embeds, "\n"))
			fmt.Fprintf(&makers, "{reflect.TypeFor[%s](), func(o vmObj, k any) any { return %s{%s} }},\n", name, name, strings.Join(append([]string{baseLit}, lits...), ", "))
		}
		out.Write(types.Bytes())
		fmt.Fprintf(&table, "{\nname: %q,\n", f.name)
		fmt.Fprintf(&table, "base: []optMethod{\n")
		for _, mt := range f.base {
			fmt.Fprintf(&table, "{%q, reflect.TypeFor[%s]()},\n", mt.name, sigType(mt))
		}
		fmt.Fprintf(&table, "},\nfeats: []optMethod{\n")
		for _, mt := range f.feats {
			fmt.Fprintf(&table, "{%q, reflect.TypeFor[%s]()},\n", mt.name, sigType(mt))
		}
		fmt.Fprintf(&table, "},\ncombos: []optCombo{\n%s},\n},\n", makers.String())
	}
	fmt.Fprintf(&table, "}\n")
	out.Write(table.Bytes())
	src, err := format.Source(out.Bytes())
	if err != nil {
		os.WriteFile("zz_optcombos.go.bad", out.Bytes(), 0o644)
		log.Fatal(err)
	}
	if err := os.WriteFile("zz_optcombos.go", src, 0o644); err != nil {
		log.Fatal(err)
	}
}
