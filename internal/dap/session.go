package dap

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type Handler interface {
	Initialize(*Session, *InitializeRequestArguments) *Capabilities
	Process(IProtocolMessage) (stop bool)
	Terminate()
}

type Session struct {
	handler Handler
	dec     *Decoder
	enc     *Encoder
	rc      io.Closer
	wc      io.Closer
	dbg     io.Writer
	errs    []error
	seq     int
	smu     *sync.Mutex
}

type SessionRequest struct {
	*Session
	*InitializeRequestArguments
	Message IProtocolMessage
}

func NewSession(r io.Reader, w io.Writer, handler Handler) *Session {
	rc, _ := r.(io.ReadCloser)
	wc, _ := w.(io.WriteCloser)
	return &Session{
		handler: handler,
		rc:      rc,
		wc:      wc,
		dec:     NewDecoder(r),
		enc:     NewEncoder(w),
		smu:     new(sync.Mutex),
	}
}

func (s *Session) Errors() []error { return s.errs }

func (s *Session) Debug(w io.Writer) { s.dbg = w }

func (s *Session) debug(dir string, msg IProtocolMessage) {
	if s.dbg == nil {
		return
	}

	b, err := json.Marshal(msg)
	if err == nil {
		fmt.Fprintf(s.dbg, "%s %s\n", dir, b)
	} else {
		fmt.Fprintf(s.dbg, "%s !%v\n", dir, err)
	}
}

func (s *Session) errorf(format string, a ...interface{}) {
	err := fmt.Errorf(format, a...)
	s.errs = append(s.errs, err)
}

func (s *Session) close() {
	if s.rc != nil {
		err := s.rc.Close()
		if err != nil {
			s.errs = append(s.errs, err)
		}
	}
	if s.wc != nil {
		err := s.wc.Close()
		if err != nil {
			s.errs = append(s.errs, err)
		}
	}
}

func (s *Session) recv() (IProtocolMessage, error) {
	m, err := s.dec.Decode()
	if err == nil {
		s.debug(">", m)
		return m, nil
	}

	if s.dbg != nil {
		fmt.Fprintf(s.dbg, "> !%v\n", err)
	}
	return nil, err
}

func (s *Session) send(msg IProtocolMessage) error {
	s.smu.Lock()
	defer s.smu.Unlock()

	s.seq++
	msg.setSeq(s.seq)

	s.debug("<", msg)
	err := s.enc.Encode(msg)
	if err == nil {
		return nil
	}

	if s.dbg != nil {
		fmt.Fprintf(s.dbg, "< !%v\n", err)
	}
	return err
}

func (s *Session) Event(event string, body EventBody) error {
	evt := new(Event)
	evt.Event = event
	evt.Body = body
	return s.send(evt)
}

func (s *Session) Respond(req *Request, success bool, message string, body ResponseBody) error {
	resp := new(Response)
	resp.RequestSeq = req.Seq
	resp.Command = req.Command
	resp.Success = success
	resp.Message = message
	resp.Body = body
	return s.send(resp)
}

func (s *Session) initialize() {
	m, err := s.recv()
	if err != nil {
		s.errorf("initialize: decode: %w", err)
		return
	}

	req, ok := m.(*Request)
	if !ok {
		s.errorf("initialize: expected a request, got %T", m)
		return
	}

	if req.Command != "initialize" {
		s.errorf("initialize: expected \"initialize\", got %q", req.Command)
		return
	}

	args, ok := req.Arguments.(*InitializeRequestArguments)
	if !ok {
		s.errorf("initialize: expected initialize request arguments, got %T", req.Arguments)
		return
	}

	caps := s.handler.Initialize(s, args)
	err = s.Respond(req, true, "Success", caps)
	if err != nil {
		s.errorf("initialize: encode: %w", err)
		return
	}
}

func (s *Session) terminate() {
	err := s.Event("terminated", new(TerminatedEventBody))
	if err != nil {
		s.errorf("terminate: encode: %w", err)
	}

	s.handler.Terminate()
}

func (s *Session) Run() {
	defer s.close()

	s.initialize()

	for {
		m, err := s.recv()
		if err != nil {
			s.errorf("loop: decode: %w", err)
			return
		}

		stop := s.handler.Process(m)
		if stop {
			break
		}
	}

	s.terminate()
}
