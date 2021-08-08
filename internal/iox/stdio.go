package iox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// TODO use net.ErrClosed when support for 1.15 is dropped
var ErrClosed = errors.New("closed")

// NewStdio returns a new Stdio listener.
func NewStdio() net.Listener {
	s := new(Stdio)
	s.mu = new(sync.Mutex)
	s.cond = sync.NewCond(s.mu)
	return s
}

// Stdio is a net.Listener that produces StdioConns.
type Stdio struct {
	state stdioState
	mu    *sync.Mutex
	cond  *sync.Cond
}

// Addr returns a StdioAddr.
func (s *Stdio) Addr() net.Addr { return StdioAddr(os.Getpid()) }

// Close closes the listener.
func (s *Stdio) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == stdioClosed {
		return ErrClosed
	}

	s.state = stdioClosed
	return nil
}

// Accept returns a StdioConn, blocking until any previous StdioConn has been
// closed.
func (s *Stdio) Accept() (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.state != stdioIdle {
		if s.state == stdioClosed {
			return nil, ErrClosed
		}

		s.cond.Wait()
	}

	s.state = stdioOpen
	return &StdioConn{s}, nil
}

type stdioState int

const (
	stdioIdle stdioState = iota
	stdioOpen
	stdioClosed
)

// StdioAddr implements net.Addr.
type StdioAddr int

// Network returns "stdio".
func (StdioAddr) Network() string { return "stdio" }

// String implements net.Addr.
func (a StdioAddr) String() string { return fmt.Sprint(int(a)) }

// StdioConn is a net.Conn that reads stdin and writes stdout.
type StdioConn struct{ s *Stdio }

// LocalAddr implements net.Conn.
func (c *StdioConn) LocalAddr() net.Addr { return StdioAddr(os.Getpid()) }

// RemoteAddr implements net.Conn.
func (c *StdioConn) RemoteAddr() net.Addr { return StdioAddr(-1) }

// Close releases the StdioConn.
func (c *StdioConn) Close() error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	c.s.state = stdioIdle
	return nil
}

// Read reads from stdin.
func (c *StdioConn) Read(b []byte) (int, error) { return os.Stdin.Read(b) }

// Write writes to stdout.
func (c *StdioConn) Write(b []byte) (int, error) { return os.Stdout.Write(b) }

// SetReadDeadline sets a read deadline on stdin.
func (c *StdioConn) SetReadDeadline(t time.Time) error { return os.Stdin.SetReadDeadline(t) }

// SetWriteDeadline sets a write deadline on stdout.
func (c *StdioConn) SetWriteDeadline(t time.Time) error { return os.Stdout.SetWriteDeadline(t) }

// SetDeadline sets a read deadline and a write deadline on stdin and stdout,
// respectively.
func (c *StdioConn) SetDeadline(t time.Time) error {
	e1 := c.SetReadDeadline(t)
	e2 := c.SetWriteDeadline(t)
	if e1 == nil {
		return e2
	}
	return e1
}
