// Native boundary of net: program errors, connections, contexts and readers
// given to net (docs/BOUNDARY.md). The DNS answers come from a fake server of
// the program (Resolver.Dial): no real DNS query is made.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
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

// seen records the kinds of things the program methods saw (a set).
var (
	seenMu sync.Mutex
	seen   = map[string]bool{}
)

func see(s string) {
	seenMu.Lock()
	seen[s] = true
	seenMu.Unlock()
}

// takeSeen gives the recorded things, sorted, and forgets them.
func takeSeen() string {
	seenMu.Lock()
	defer seenMu.Unlock()
	var s []string
	for k := range seen {
		s = append(s, k)
	}
	sort.Strings(s)
	seen = map[string]bool{}
	return strings.Join(s, " ")
}

// tmoErr is a timeout error (value receivers).
type tmoErr struct{ s string }

func (e tmoErr) Error() string   { return "tmoErr " + e.s }
func (e tmoErr) Timeout() bool   { return true }
func (e tmoErr) Temporary() bool { return false }

// tmpErr is a temporary error (pointer receivers).
type tmpErr struct{ s string }

func (e *tmpErr) Error() string   { return "tmpErr " + e.s }
func (e *tmpErr) Timeout() bool   { return false }
func (e *tmpErr) Temporary() bool { return true }

// onlyTimeout has Timeout but not Temporary.
type onlyTimeout struct{}

func (onlyTimeout) Error() string { return "onlyTimeout" }
func (onlyTimeout) Timeout() bool { return true }

// gErr is a generic error with both methods.
type gErr[T any] struct{ v T }

func (e gErr[T]) Error() string   { return fmt.Sprintf("gErr %v", e.v) }
func (e gErr[T]) Timeout() bool   { return true }
func (e gErr[T]) Temporary() bool { return true }

// plainErr has no method of net.Error.
type plainErr struct{}

func (plainErr) Error() string { return "plainErr" }

// encName encodes a DNS name.
func encName(name string) []byte {
	var b []byte
	for _, l := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

// dnsAnswer is the fake DNS server: it answers the query q.
func dnsAnswer(q []byte) []byte {
	if len(q) < 12 {
		return nil
	}
	i := 12
	var labels []string
	for i < len(q) && q[i] != 0 {
		l := int(q[i])
		if i+1+l > len(q) {
			return nil
		}
		labels = append(labels, string(q[i+1:i+1+l]))
		i += 1 + l
	}
	i++
	if i+4 > len(q) {
		return nil
	}
	qtype := int(q[i])<<8 | int(q[i+1])
	name := strings.ToLower(strings.Join(labels, ".") + ".")
	see(fmt.Sprintf("q%d", qtype))
	resp := []byte{q[0], q[1], 0x85, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}
	resp = append(resp, q[12:i+4]...)
	n := 0
	rr := func(typ int, data []byte) {
		resp = append(resp, 0xc0, 12, byte(typ>>8), byte(typ), 0, 1, 0, 0, 0, 60, byte(len(data)>>8), byte(len(data)))
		resp = append(resp, data...)
		n++
	}
	switch {
	case strings.HasPrefix(name, "nx."):
		resp[3] |= 3 // NXDOMAIN
	case qtype == 1: // A
		rr(1, []byte{192, 0, 2, 1})
	case qtype == 28: // AAAA: no answer
	case qtype == 5: // CNAME
		rr(5, encName("canon.example."))
	case qtype == 15: // MX
		rr(15, append([]byte{0, 10}, encName("mail.example.")...))
	case qtype == 2: // NS
		rr(2, encName("ns1.example."))
	case qtype == 16: // TXT
		rr(16, append([]byte{9}, "hello txt"...))
	case qtype == 33: // SRV
		rr(33, append([]byte{0, 1, 0, 2, 0x13, 0x88}, encName("srv.example.")...))
	case qtype == 12: // PTR
		rr(12, encName("ptr.example."))
	default:
		resp[3] |= 3
	}
	resp[7] = byte(n)
	return resp
}

// dnsConn is the connection of the fake DNS server (a stream conn).
type dnsConn struct {
	mu     sync.Mutex
	stream bool
	resp   []byte
}

func (c *dnsConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	q := b
	if c.stream {
		q = b[2:]
	}
	r := dnsAnswer(q)
	if c.stream {
		r = append([]byte{byte(len(r) >> 8), byte(len(r))}, r...)
	}
	c.resp = append(c.resp, r...)
	return len(b), nil
}

func (c *dnsConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.resp) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.resp)
	c.resp = c.resp[n:]
	return n, nil
}

func (c *dnsConn) Close() error                       { return nil }
func (c *dnsConn) LocalAddr() net.Addr                { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5353} }
func (c *dnsConn) RemoteAddr() net.Addr               { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53} }
func (c *dnsConn) SetDeadline(t time.Time) error      { return nil }
func (c *dnsConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *dnsConn) SetWriteDeadline(t time.Time) error { return nil }

// pktConn is a net.PacketConn (value receivers for the packet methods).
type pktConn struct{ *dnsConn }

func (c pktConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, err := c.Read(p)
	return n, c.RemoteAddr(), err
}
func (c pktConn) WriteTo(p []byte, a net.Addr) (int, error) { return c.Write(p) }

// gpktConn is a generic net.PacketConn.
type gpktConn[T any] struct {
	*dnsConn
	tag T
}

func (c gpktConn[T]) ReadFrom(p []byte) (int, net.Addr, error) {
	n, err := c.Read(p)
	return n, c.RemoteAddr(), err
}
func (c gpktConn[T]) WriteTo(p []byte, a net.Addr) (int, error) { return c.Write(p) }

// fakeDial gives the Dial function of a Resolver that uses the fake server.
func fakeDial(kind string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		see(kind + ":" + network)
		switch kind {
		case "stream":
			return &dnsConn{stream: true}, nil
		case "packet":
			return pktConn{&dnsConn{}}, nil
		case "generic":
			return gpktConn[int]{&dnsConn{}, 1}, nil
		case "timeout":
			return nil, tmoErr{"dial"}
		case "temporary":
			return nil, &tmpErr{"dial"}
		case "generr":
			return nil, gErr[string]{"dial"}
		}
		return nil, plainErr{}
	}
}

// dnsErr describes an error without the server address.
func dnsErr(err error) string {
	if err == nil {
		return "<nil>"
	}
	var de *net.DNSError
	if errors.As(err, &de) {
		return fmt.Sprintf("DNSError{Err %q Name %q timeout %v temporary %v notfound %v}", de.Err, de.Name, de.IsTimeout, de.IsTemporary, de.IsNotFound)
	}
	return err.Error()
}

// altCtx gives a value for the key of the alternative resolver of net.
type altCtx struct {
	context.Context
	alt any
}

func (c altCtx) Value(k any) any {
	if fmt.Sprintf("%T", k) == "nettrace.LookupIPAltResolverKey" {
		see("altCtx.Value")
		return c.alt
	}
	return c.Context.Value(k)
}

// altFn is a named type over the function type of the alternative resolver.
type altFn func(context.Context, string, string) ([]net.IPAddr, error)

// syscallReader is a reader with SyscallConn that gives an error.
type syscallReader struct{ s *strings.Reader }

func (r syscallReader) Read(p []byte) (int, error) { return r.s.Read(p) }
func (r syscallReader) SyscallConn() (syscall.RawConn, error) {
	see("syscallReader.SyscallConn")
	return nil, errors.New("no raw conn")
}

// rawConn is a syscall.RawConn of the program that refuses to read.
type rawConn struct{}

func (rawConn) Control(f func(fd uintptr)) error { return errors.New("no control") }
func (rawConn) Read(f func(fd uintptr) bool) error {
	see("rawConn.Read")
	return errors.New("no read")
}
func (rawConn) Write(f func(fd uintptr) bool) error { return errors.New("no write") }

// rawReader is a reader whose SyscallConn gives rawConn (pointer receivers).
type rawReader struct{ s *strings.Reader }

func (r *rawReader) Read(p []byte) (int, error) { return r.s.Read(p) }
func (r *rawReader) SyscallConn() (syscall.RawConn, error) {
	see("rawReader.SyscallConn")
	return rawConn{}, nil
}

// fileReader is a reader over a file, with the SyscallConn of the file.
type fileReader[T any] struct {
	f   *os.File
	tag T
}

func (r fileReader[T]) Read(p []byte) (int, error) { see("fileReader.Read"); return r.f.Read(p) }
func (r fileReader[T]) SyscallConn() (syscall.RawConn, error) {
	see(fmt.Sprintf("fileReader[%v].SyscallConn", r.tag))
	return r.f.SyscallConn()
}

// plainReader is a reader without SyscallConn.
type plainReader struct{ s *strings.Reader }

func (r *plainReader) Read(p []byte) (int, error) { return r.s.Read(p) }

func main() {
	// row: net (*OpError).Timeout assert x.(net.timeout)
	// row: net (*OpError).Temporary assert x.(net.temporary)
	probe("OpError", func() {
		var ie error = tmoErr{"i"}
		errs := []error{tmoErr{"a"}, &tmpErr{"b"}, onlyTimeout{}, gErr[int]{1}, plainErr{}, ie, []error{&tmpErr{"s"}}[0]}
		for i, e := range errs {
			op := &net.OpError{Op: "read", Net: "tcp", Err: e}
			sys := &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", e)}
			acc := &net.OpError{Op: "accept", Net: "tcp", Err: e}
			out("%d: %v timeout %v temporary %v; syscall timeout %v temporary %v; accept temporary %v",
				i, op, op.Timeout(), op.Temporary(), sys.Timeout(), sys.Temporary(), acc.Temporary())
		}
	})

	lookups := func(r *net.Resolver, ctx context.Context) {
		addrs, err := r.LookupHost(ctx, "ok.example.")
		out("  LookupHost %v %s", addrs, dnsErr(err))
		ips, err := r.LookupIPAddr(ctx, "ok.example.")
		out("  LookupIPAddr %v %s", ips, dnsErr(err))
		ipl, err := r.LookupIP(ctx, "ip4", "ok.example.")
		out("  LookupIP %v %s", ipl, dnsErr(err))
		nips, err := r.LookupNetIP(ctx, "ip", "ok.example.")
		out("  LookupNetIP %v %s", nips, dnsErr(err))
		// the name depends on which of the A and AAAA answers comes first
		_, err = r.LookupCNAME(ctx, "ok.example.")
		out("  LookupCNAME %s", dnsErr(err))
		mx, err := r.LookupMX(ctx, "ok.example.")
		for _, m := range mx {
			out("  LookupMX %s %d", m.Host, m.Pref)
		}
		out("  LookupMX %d %s", len(mx), dnsErr(err))
		ns, err := r.LookupNS(ctx, "example.")
		for _, n := range ns {
			out("  LookupNS %s", n.Host)
		}
		out("  LookupNS %d %s", len(ns), dnsErr(err))
		txt, err := r.LookupTXT(ctx, "ok.example.")
		out("  LookupTXT %q %s", txt, dnsErr(err))
		cn, srv, err := r.LookupSRV(ctx, "svc", "tcp", "ok.example.")
		for _, s := range srv {
			out("  LookupSRV %s %d %d %d", s.Target, s.Port, s.Priority, s.Weight)
		}
		out("  LookupSRV %q %d %s", cn, len(srv), dnsErr(err))
		names, err := r.LookupAddr(ctx, "192.0.2.7")
		out("  LookupAddr %v %s", names, dnsErr(err))
		addrs, err = r.LookupHost(ctx, "nx.example.")
		out("  LookupHost nx %v %s", addrs, dnsErr(err))
		port, err := r.LookupPort(ctx, "tcp", "https")
		out("  LookupPort %d %s", port, dnsErr(err))
		port, err = r.LookupPort(ctx, "tcp", "no-such-service-xyz")
		out("  LookupPort unknown %d %s", port, dnsErr(err))
	}
	// row: net (*Resolver).LookupAddr,(*Resolver).LookupCNAME,(*Resolver).LookupHost,(*Resolver).LookupIP,(*Resolver).LookupIPAddr,(*Resolver).LookupMX,(*Resolver).LookupNS,(*Resolver).LookupNetIP,(*Resolver).LookupSRV,(*Resolver).LookupTXT assert x.(net.PacketConn)
	// row: net (*Resolver).LookupAddr,(*Resolver).LookupCNAME,(*Resolver).LookupHost,(*Resolver).LookupIP,(*Resolver).LookupIPAddr,(*Resolver).LookupMX,(*Resolver).LookupNS,(*Resolver).LookupNetIP,(*Resolver).LookupSRV,(*Resolver).LookupTXT,(*Resolver).LookupPort assert x.(net.Error)
	resolver := func(kind string) func() {
		return func() {
			r := &net.Resolver{PreferGo: true, Dial: fakeDial(kind)}
			lookups(r, context.Background())
			out("  seen: %s", takeSeen())
		}
	}
	probe("Resolver/stream", resolver("stream"))
	probe("Resolver/packet", resolver("packet"))
	probe("Resolver/generic", resolver("generic"))
	probe("Resolver/timeout", resolver("timeout"))
	probe("Resolver/temporary", resolver("temporary"))
	probe("Resolver/generr", resolver("generr"))
	probe("Resolver/plain", resolver("plain"))
	// row: net (*Resolver).LookupHost,(*Resolver).LookupIP,(*Resolver).LookupIPAddr,(*Resolver).LookupNetIP assert x.(T), T unnamed
	probe("Resolver/alt", func() {
		r := &net.Resolver{PreferGo: true, Dial: fakeDial("stream")}
		alt := func(ctx context.Context, network, host string) ([]net.IPAddr, error) {
			see("alt " + network + " " + host)
			return []net.IPAddr{{IP: net.IPv4(203, 0, 113, 5)}}, nil
		}
		for i, v := range []any{alt, altFn(alt), nil} {
			ctx := altCtx{context.Background(), v}
			addrs, err := r.LookupHost(ctx, fmt.Sprintf("h%d.example.", i))
			out("%d: LookupHost %v %s", i, addrs, dnsErr(err))
			ips, err := r.LookupIPAddr(ctx, fmt.Sprintf("i%d.example.", i))
			out("%d: LookupIPAddr %v %s", i, ips, dnsErr(err))
			ipl, err := r.LookupIP(ctx, "ip", fmt.Sprintf("j%d.example.", i))
			out("%d: LookupIP %v %s", i, ipl, dnsErr(err))
			nips, err := r.LookupNetIP(ctx, "ip", fmt.Sprintf("k%d.example.", i))
			out("%d: LookupNetIP %v %s", i, nips, dnsErr(err))
			out("%d: seen: %s", i, takeSeen())
		}
	})
	// row: net LookupAddr,LookupCNAME,LookupHost,LookupIP,LookupMX,LookupNS,LookupSRV,LookupTXT,ResolveIPAddr,ResolveTCPAddr,ResolveUDPAddr,Dial,DialTimeout,Listen,ListenPacket,(*Dialer).Dial,(*Dialer).DialContext,(*ListenConfig).Listen,(*ListenConfig).ListenPacket assert x.(net.PacketConn)
	// row: net LookupAddr,LookupCNAME,LookupHost,LookupIP,LookupMX,LookupNS,LookupSRV,LookupTXT,ResolveIPAddr,ResolveTCPAddr,ResolveUDPAddr,Dial,DialTimeout,Listen,ListenPacket,(*Dialer).Dial,(*Dialer).DialContext,(*ListenConfig).Listen,(*ListenConfig).ListenPacket,LookupPort assert x.(net.Error)
	// row: net LookupAddr,LookupCNAME,LookupHost,LookupIP,LookupMX,LookupNS,LookupSRV,LookupTXT,ResolveIPAddr,ResolveTCPAddr,ResolveUDPAddr,Dial,DialTimeout,Listen,ListenPacket,(*Dialer).Dial,(*Dialer).DialContext,(*ListenConfig).Listen,(*ListenConfig).ListenPacket,(*Resolver).LookupAddr,(*Resolver).LookupCNAME,(*Resolver).LookupMX,(*Resolver).LookupNS,(*Resolver).LookupSRV,(*Resolver).LookupTXT assert x.(T), T unnamed
	defaultResolver := func(kind string) func() {
		return func() {
			old := net.DefaultResolver
			defer func() { net.DefaultResolver = old }()
			net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: fakeDial(kind)}
			addrs, err := net.LookupHost("ok.example.")
			out("  LookupHost %v %s", addrs, dnsErr(err))
			ipl, err := net.LookupIP("ok.example.")
			out("  LookupIP %v %s", ipl, dnsErr(err))
			_, err = net.LookupCNAME("ok.example.")
			out("  LookupCNAME %s", dnsErr(err))
			mx, err := net.LookupMX("ok.example.")
			out("  LookupMX %d %s", len(mx), dnsErr(err))
			ns, err := net.LookupNS("example.")
			out("  LookupNS %d %s", len(ns), dnsErr(err))
			txt, err := net.LookupTXT("ok.example.")
			out("  LookupTXT %q %s", txt, dnsErr(err))
			_, srv, err := net.LookupSRV("svc", "tcp", "ok.example.")
			out("  LookupSRV %d %s", len(srv), dnsErr(err))
			names, err := net.LookupAddr("192.0.2.9")
			out("  LookupAddr %v %s", names, dnsErr(err))
			port, err := net.LookupPort("tcp", "https")
			out("  LookupPort %d %s", port, dnsErr(err))
			ia, err := net.ResolveIPAddr("ip4", "ok.example.")
			out("  ResolveIPAddr %v %s", ia, dnsErr(err))
			ta, err := net.ResolveTCPAddr("tcp4", "ok.example.:80")
			out("  ResolveTCPAddr %v %s", ta, dnsErr(err))
			ua, err := net.ResolveUDPAddr("udp4", "ok.example.:53")
			out("  ResolveUDPAddr %v %s", ua, dnsErr(err))
			// nx names: the lookup fails, nothing is dialed or bound.
			_, err = net.Dial("tcp", "nx.example.:80")
			out("  Dial %s", dnsErr(err))
			_, err = net.DialTimeout("tcp", "nx.example.:80", time.Second)
			out("  DialTimeout %s", dnsErr(err))
			_, err = net.Listen("tcp", "nx.example.:0")
			out("  Listen %s", dnsErr(err))
			_, err = net.ListenPacket("udp", "nx.example.:0")
			out("  ListenPacket %s", dnsErr(err))
			d := &net.Dialer{Resolver: &net.Resolver{PreferGo: true, Dial: fakeDial("generic")}}
			_, err = d.Dial("tcp", "nx.example.:80")
			out("  Dialer.Dial %s", dnsErr(err))
			_, err = d.DialContext(context.Background(), "tcp", "nx.example.:80")
			out("  Dialer.DialContext %s", dnsErr(err))
			var lc net.ListenConfig
			_, err = lc.Listen(context.Background(), "tcp", "nx.example.:0")
			out("  ListenConfig.Listen %s", dnsErr(err))
			_, err = lc.ListenPacket(context.Background(), "udp", "nx.example.:0")
			out("  ListenConfig.ListenPacket %s", dnsErr(err))
			out("  seen: %s", takeSeen())
		}
	}
	probe("DefaultResolver/stream", defaultResolver("stream"))
	probe("DefaultResolver/packet", defaultResolver("packet"))
	probe("DefaultResolver/timeout", defaultResolver("timeout"))
	probe("DefaultResolver/alt", func() {
		// The context of DialContext and Listen can give the alternative resolver.
		old := net.DefaultResolver
		defer func() { net.DefaultResolver = old }()
		net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: fakeDial("stream")}
		altErr := func(ctx context.Context, network, host string) ([]net.IPAddr, error) {
			see("alt " + host)
			return nil, errors.New("alt resolver refused " + host)
		}
		for i, v := range []any{altErr, altFn(altErr)} {
			ctx := altCtx{context.Background(), v}
			d := &net.Dialer{}
			_, err := d.DialContext(ctx, "tcp", "nx.example.:80")
			out("alt %d: Dialer.DialContext %s", i, dnsErr(err))
			var lc net.ListenConfig
			_, err = lc.Listen(ctx, "tcp", "nx.example.:0")
			out("alt %d: ListenConfig.Listen %s", i, dnsErr(err))
			_, err = lc.ListenPacket(ctx, "udp", "nx.example.:0")
			out("alt %d: ListenConfig.ListenPacket %s", i, dnsErr(err))
			out("alt %d: seen: %s", i, takeSeen())
		}
	})
	// row: net (*TCPConn).ReadFrom assert x.(syscall.Conn)
	probe("TCPConn.ReadFrom", func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			out("listen failed (no loopback here)")
			return
		}
		defer ln.Close()
		f, err := os.CreateTemp("", "readfrom")
		if err != nil {
			out("CreateTemp failed")
			return
		}
		defer os.Remove(f.Name())
		defer f.Close()
		f.WriteString("file-data")
		readers := []func() io.Reader{
			func() io.Reader { return &plainReader{strings.NewReader("plain")} },
			func() io.Reader { return syscallReader{strings.NewReader("syscall")} },
			func() io.Reader { return &rawReader{strings.NewReader("raw")} },
			func() io.Reader { f.Seek(0, 0); return fileReader[int]{f, 1} },
			func() io.Reader { f.Seek(0, 0); return &io.LimitedReader{R: fileReader[string]{f, "l"}, N: 4} },
			func() io.Reader { return &io.LimitedReader{R: syscallReader{strings.NewReader("limited")}, N: 3} },
		}
		for i, mk := range readers {
			got := make(chan string, 1)
			go func() {
				c, err := ln.Accept()
				if err != nil {
					got <- "accept: " + err.Error()
					return
				}
				b, _ := io.ReadAll(c)
				c.Close()
				got <- string(b)
			}()
			c, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				out("%d: dial failed", i)
				continue
			}
			n, err := c.(*net.TCPConn).ReadFrom(mk())
			c.Close()
			// The error text has the ports: print only if there is one.
			out("%d: ReadFrom %d error %v, peer got %q, seen: %s", i, n, err != nil, <-got, takeSeen())
		}
	})
}
