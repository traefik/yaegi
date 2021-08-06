package dap

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Handler handles DAP events.
type Handler interface {
	// Called when the DAP session begins.
	Initialize(*Session, *InitializeRequestArguments) *Capabilities

	// Called for each received DAP message. Returning true will terminate the
	// session.
	Process(IProtocolMessage) (stop bool)

	// Called when the DAP session ends.
	Terminate()
}

// Session handles the low-level mechanics of a DAP session.
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

// NewSession returns a new Session. The session reads messages from the reader
// and writes messages to the writer. If the reader or writer are io.Closers,
// they will be closed when the session terminates.
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

// Errors returns any errors that occurred during the session.
func (s *Session) Errors() []error { return s.errs }

// Debug sets the debug writer. If the debug writer is non-null, sent and
// received DAP messages will be written to it.
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

// Errorf logs an error.
func (s *Session) Errorf(format string, a ...interface{}) {
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

	err := s.enc.Encode(msg)
	if err == nil {
		s.debug("<", msg)
		return nil
	}

	if s.dbg != nil {
		fmt.Fprintf(s.dbg, "< !%v\n", err)
	}
	return err
}

// Event sends a DAP event.
func (s *Session) Event(event string, body EventBody) error {
	evt := new(Event)
	evt.Event = event
	evt.Body = body
	return s.send(evt)
}

// Respond sends a DAP response to the given request.
func (s *Session) Respond(req *Request, success bool, message string, body ResponseBody) error {
	resp := new(Response)
	resp.RequestSeq = req.Seq
	resp.Command = req.Command
	resp.Success = success
	resp.Message = Str(message)
	resp.Body = body
	return s.send(resp)
}

func (s *Session) initialize() {
	m, err := s.recv()
	if err != nil {
		s.Errorf("initialize: decode: %w", err)
		return
	}

	req, ok := m.(*Request)
	if !ok {
		s.Errorf("initialize: expected a request, got %T", m)
		return
	}

	if req.Command != "initialize" {
		s.Errorf("initialize: expected \"initialize\", got %q", req.Command)
		return
	}

	args := req.Arguments.(*InitializeRequestArguments)
	caps := s.handler.Initialize(s, args)
	err = s.Respond(req, true, "Success", caps)
	if err != nil {
		s.Errorf("initialize: encode: %w", err)
		return
	}
}

func (s *Session) terminate() {
	err := s.Event("terminated", new(TerminatedEventBody))
	if err != nil {
		s.Errorf("terminate: encode: %w", err)
	}

	s.handler.Terminate()
}

// Run starts the session. Run blocks until the session is terminated.
func (s *Session) Run() {
	defer s.close()

	s.initialize()

	for {
		m, err := s.recv()
		if err != nil {
			s.Errorf("loop: decode: %w", err)
			return
		}

		stop := s.handler.Process(m)
		if stop {
			break
		}
	}

	s.terminate()
}
