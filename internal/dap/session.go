package dap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrStop is the error returned by a handler to indicate that the session
// should terminate.
var ErrStop = errors.New("stop")

// Handler handles DAP events.
type Handler interface {
	// Called when the DAP session begins.
	Initialize(*Session, *InitializeRequestArguments) (*Capabilities, error)

	// Called for each received DAP message. Returning an error will terminate
	// the session.
	Process(IProtocolMessage) error

	// Called when the DAP session ends.
	Terminate()
}

// Session handles the low-level mechanics of a DAP session.
type Session struct {
	handler Handler
	dec     *Decoder
	enc     *Encoder
	dbg     io.Writer
	seq     int
	smu     *sync.Mutex
}

// NewSession returns a new Session. The session reads messages from the reader
// and writes messages to the writer. The caller is responsible for closing the
// reader and writer.
func NewSession(r io.Reader, w io.Writer, handler Handler) *Session {
	return &Session{
		handler: handler,
		dec:     NewDecoder(r),
		enc:     NewEncoder(w),
		smu:     new(sync.Mutex),
	}
}

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

func (s *Session) initialize() error {
	m, err := s.recv()
	if err != nil {
		return fmt.Errorf("initialize: decode: %w", err)
	}

	req, ok := m.(*Request)
	if !ok {
		return fmt.Errorf("initialize: expected a request, got %T", m)
	}

	if req.Command != "initialize" {
		return fmt.Errorf("initialize: expected \"initialize\", got %q", req.Command)
	}

	args := req.Arguments.(*InitializeRequestArguments)
	caps, err := s.handler.Initialize(s, args)
	if err != nil {
		return err
	}

	err = s.Respond(req, true, "Success", caps)
	if err != nil {
		return fmt.Errorf("initialize: encode: %w", err)
	}

	return nil
}

func (s *Session) terminate() error {
	err := s.Event("terminated", new(TerminatedEventBody))
	s.handler.Terminate()
	return err
}

// Run starts the session. Run blocks until the session is terminated.
func (s *Session) Run() error {
	err := s.initialize()
	if err != nil {
		return err
	}

	for {
		m, err := s.recv()
		if err != nil {
			return fmt.Errorf("loop: decode: %w", err)
		}

		err = s.handler.Process(m)
		if errors.Is(err, ErrStop) {
			break
		} else if err != nil {
			return err
		}
	}

	return s.terminate()
}
