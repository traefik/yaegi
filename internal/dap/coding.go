package dap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ErrInvalidContentLength is the error returned by Decoder.Decode if the
// Content-Length header has an invalid value.
var ErrInvalidContentLength = errors.New("invalid content length")

// ErrMissingContentLength is the error returned by Decoder.Decode if the
// Content-Length header is missing.
var ErrMissingContentLength = errors.New("missing content length")

// Encoder marshalls DAP messages.
type Encoder struct {
	w io.Writer
}

// NewEncoder returns a new encoder that uses the given writer.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w}
}

// Encode marshalls a DAP message.
//
// Encode will fail if seq has not been set. Encode will fail if the
// arguments/body of a request, response, or event does not match its
// command/event field.
func (c *Encoder) Encode(msg IProtocolMessage) error {
	json, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Write to a buffer so the final write is atomic, as long as the underling
	// writer is atomic
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "Content-Length: %d\r\n", len(json))
	fmt.Fprintf(buf, "\r\n")
	buf.Write(json)

	_, err = buf.WriteTo(c.w)
	return err
}

// Decoder unmarshalls DAP messages.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder returns a new decoder that uses the given reader.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{bufio.NewReader(r)}
}

// Decode unmarshalls a DAP message. Decode guarantees that the arguments/body
// of a successfully decoded request, response, or event will match the
// command/event field.
//
// Decode returns ErrInvalidContentLength if the Content-Length header has an
// invalid value. Decode returns ErrMissingContentLength if the Content-Length
// header is missing.
func (c *Decoder) Decode() (IProtocolMessage, error) {
	buf := new(bytes.Buffer)
	for {
		line, err := c.r.ReadSlice('\r')
		if errors.Is(err, bufio.ErrBufferFull) {
			buf.Write(line)
			continue
		} else if err != nil {
			return nil, err
		}

		buf.Write(line)

	lf:
		b, err := c.r.ReadByte()
		if err != nil {
			return nil, err
		}

		buf.WriteByte(b)
		if b == '\r' {
			goto lf
		} else if b != '\n' {
			continue
		}

		if bytes.HasSuffix(buf.Bytes(), []byte("\r\n\r\n")) {
			break
		}
	}

	want := -1
	headers := bytes.Split(buf.Bytes(), []byte("\r\n"))
	for _, header := range headers {
		i := bytes.IndexByte(header, ':')
		var key, value []byte
		if i < 0 {
			key = bytes.TrimSpace(header)
		} else {
			key = bytes.TrimSpace(header[:i])
			value = bytes.TrimSpace(header[i+1:])
		}

		if bytes.EqualFold(key, []byte("Content-Length")) {
			if len(value) == 0 {
				return nil, ErrInvalidContentLength
			}

			v, err := strconv.ParseInt(string(value), 10, 32)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidContentLength, err)
			}
			if v < 0 {
				return nil, ErrInvalidContentLength
			}

			want = int(v)
			break
		}
	}

	payload := make([]byte, want)
	_, err := io.ReadFull(c.r, payload)
	if err != nil {
		return nil, err
	}

	pm := new(ProtocolMessage)
	err = json.Unmarshal(payload, pm)
	if err != nil {
		return nil, err
	}

	var m IProtocolMessage
	switch pm.Type {
	case ProtocolMessageType_Request:
		m = new(Request)
	case ProtocolMessageType_Response:
		m = new(Response)
	case ProtocolMessageType_Event:
		m = new(Event)
	default:
		return nil, fmt.Errorf("unrecognized message type %q", pm.Type)
	}

	err = json.Unmarshal(payload, m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// IProtocolMessage is a DAP protocol message.
type IProtocolMessage interface {
	prepareForEncoding() error
	setSeq(int)
}

func (m *ProtocolMessage) setSeq(s int) { m.Seq = s }

func (m *ProtocolMessage) prepareForEncoding() error {
	if m.Seq == 0 {
		return errors.New("seq unset")
	}
	return nil
}

// MarshalJSON marshalls the request to JSON.
func (m *Request) MarshalJSON() ([]byte, error) {
	m.Type = ProtocolMessageType_Request

	hasCmd := m.Command != ""
	hasArgs := m.Arguments != nil
	switch {
	case !hasCmd && !hasArgs:
		return nil, errors.New("command and arguments unset")
	case !hasCmd:
		m.Command = m.Arguments.requestType()
	case !hasArgs:
		// ok
	case m.Command != m.Arguments.requestType():
		return nil, fmt.Errorf("command is %q but arguments is %q", m.Command, m.Arguments.requestType())
	}

	err := m.ProtocolMessage.prepareForEncoding()
	if err != nil {
		return nil, err
	}

	type T Request
	return json.Marshal((*T)(m))
}

// MarshalJSON marshalls the response to JSON.
func (m *Response) MarshalJSON() ([]byte, error) {
	m.Type = ProtocolMessageType_Response

	hasCmd := m.Command != ""
	hasBody := m.Body != nil
	switch {
	case !hasCmd && !hasBody:
		return nil, errors.New("command and body unset")
	case !hasCmd:
		m.Command = m.Body.responseType()
	case !hasBody:
		// ok
	case m.Command != m.Body.responseType():
		return nil, fmt.Errorf("command is %q but body is %q", m.Command, m.Body.responseType())
	}

	if m.RequestSeq == 0 {
		return nil, errors.New("request_seq unset")
	}

	err := m.ProtocolMessage.prepareForEncoding()
	if err != nil {
		return nil, err
	}

	type T Response
	return json.Marshal((*T)(m))
}

// MarshalJSON marshalls the event to JSON.
func (m *Event) MarshalJSON() ([]byte, error) {
	m.Type = ProtocolMessageType_Event

	hasEvt := m.Event != ""
	hasBody := m.Body != nil
	switch {
	case !hasEvt && !hasBody:
		return nil, errors.New("event and body unset")
	case !hasEvt:
		m.Event = m.Body.eventType()
	case !hasBody:
		// ok
	case m.Event != m.Body.eventType():
		return nil, fmt.Errorf("event is %q but body is %q", m.Event, m.Body.eventType())
	}

	err := m.ProtocolMessage.prepareForEncoding()
	if err != nil {
		return nil, err
	}

	type T Event
	return json.Marshal((*T)(m))
}

// UnmarshalJSON unmarshalls the request from JSON.
func (m *Request) UnmarshalJSON(b []byte) error {
	var x struct{ Command string }
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	m.Arguments, err = newRequest(x.Command)
	if err != nil {
		return err
	}

	type T Request
	return json.Unmarshal(b, (*T)(m))
}

// UnmarshalJSON unmarshalls the response from JSON.
func (m *Response) UnmarshalJSON(b []byte) error {
	var x struct{ Command string }
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	m.Body, err = newResponse(x.Command)
	if err != nil {
		return err
	}

	type T Response
	return json.Unmarshal(b, (*T)(m))
}

// UnmarshalJSON unmarshalls the event from JSON.
func (m *Event) UnmarshalJSON(b []byte) error {
	var x struct{ Event string }
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	m.Body, err = newEvent(m.Event)
	if err != nil {
		return err
	}

	type T Event
	return json.Unmarshal(b, (*T)(m))
}

type (
	ConfigurationDoneArguments struct{} //nolint:revive
	LoadedSourcesArguments     struct{} //nolint:revive
)
