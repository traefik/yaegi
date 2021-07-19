package iox

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

func NewStdio() net.Listener {
	s := new(Stdio)
	s.mu = new(sync.Mutex)
	s.cond = sync.NewCond(s.mu)
	return s
}

type Stdio struct {
	state stdioState
	mu    *sync.Mutex
	cond  *sync.Cond
}

func (s *Stdio) Addr() net.Addr { return StdioAddr(os.Getpid()) }

func (s *Stdio) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == stdioClosed {
		return net.ErrClosed
	}

	s.state = stdioClosed
	return nil
}

func (s *Stdio) Accept() (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.state != stdioIdle {
		if s.state == stdioClosed {
			return nil, net.ErrClosed
		}

		s.cond.Wait()
	}

	return &StdioConn{s}, nil
}

type stdioState int

const (
	stdioIdle stdioState = iota
	stdioOpen
	stdioClosed
)

type StdioAddr int

func (StdioAddr) Network() string  { return "stdio" }
func (a StdioAddr) String() string { return fmt.Sprint(int(a)) }

type StdioConn struct{ s *Stdio }

func (c *StdioConn) LocalAddr() net.Addr  { return StdioAddr(os.Getpid()) }
func (c *StdioConn) RemoteAddr() net.Addr { return StdioAddr(-1) }

func (c *StdioConn) Close() error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	c.s.state = stdioIdle
	return nil
}

func (c *StdioConn) Read(b []byte) (int, error)  { return os.Stdin.Read(b) }
func (c *StdioConn) Write(b []byte) (int, error) { return os.Stdout.Write(b) }

func (c *StdioConn) SetReadDeadline(t time.Time) error  { return os.Stdin.SetReadDeadline(t) }
func (c *StdioConn) SetWriteDeadline(t time.Time) error { return os.Stdout.SetWriteDeadline(t) }

func (c *StdioConn) SetDeadline(t time.Time) error {
	e1 := c.SetReadDeadline(t)
	e2 := c.SetWriteDeadline(t)
	if e1 == nil {
		return e2
	}
	return e1
}
