package dap

import (
	"net"
)

type Server struct {
	l       net.Listener
	handler Handler
}

func NewServer(l net.Listener, handler Handler) *Server {
	return &Server{l, handler}
}

func (s *Server) Accept() (*Session, error) {
	conn, err := s.l.Accept()
	if err != nil {
		return nil, err
	}

	return NewSession(conn, conn, s.handler), nil
}
