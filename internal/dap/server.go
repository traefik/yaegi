package dap

import (
	"net"
)

// Server is a DAP server that accepts connections and starts DAP sessions.
type Server struct {
	l       net.Listener
	handler Handler
}

// NewServer returns a new DAP server that accepts connections and starts DAP
// sessions.
func NewServer(l net.Listener, handler Handler) *Server {
	return &Server{l, handler}
}

// Accept accepts a connection from the listener and returns a new session that
// reads and writes to the connection.
func (s *Server) Accept() (*Session, net.Conn, error) {
	conn, err := s.l.Accept()
	if err != nil {
		return nil, nil, err
	}

	return NewSession(conn, conn, s.handler), conn, nil
}
