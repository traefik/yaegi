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

var ErrInvalidContentLength = errors.New("invalid content length")

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w}
}

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

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{bufio.NewReader(r)}
}

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

func (m *Request) MarshalJSON() ([]byte, error) {
	m.Type = ProtocolMessageType_Request

	hasCmd := m.Command != ""
	hasArgs := m.Arguments != nil
	if !hasCmd && !hasArgs {
		return nil, errors.New("command and arguments unset")
	} else if !hasCmd {
		m.Command = m.Arguments.requestType()
	} else if !hasArgs {
		// ok
	} else if m.Command != m.Arguments.requestType() {
		return nil, fmt.Errorf("command is %q but arguments is %q", m.Command, m.Arguments.requestType())
	}

	err := m.ProtocolMessage.prepareForEncoding()
	if err != nil {
		return nil, err
	}

	type T Request
	return json.Marshal((*T)(m))
}

func (m *Response) MarshalJSON() ([]byte, error) {
	m.Type = ProtocolMessageType_Response

	hasCmd := m.Command != ""
	hasBody := m.Body != nil
	if !hasCmd && !hasBody {
		return nil, errors.New("command and body unset")
	} else if !hasCmd {
		m.Command = m.Body.responseType()
	} else if !hasBody {
		// ok
	} else if m.Command != m.Body.responseType() {
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

func (m *Event) MarshalJSON() ([]byte, error) {
	m.Type = ProtocolMessageType_Event

	hasEvt := m.Event != ""
	hasBody := m.Body != nil
	if !hasEvt && !hasBody {
		return nil, errors.New("event and body unset")
	} else if !hasEvt {
		m.Event = m.Body.eventType()
	} else if !hasBody {
		// ok
	} else if m.Event != m.Body.eventType() {
		return nil, fmt.Errorf("event is %q but body is %q", m.Event, m.Body.eventType())
	}

	err := m.ProtocolMessage.prepareForEncoding()
	if err != nil {
		return nil, err
	}

	type T Event
	return json.Marshal((*T)(m))
}

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

type ConfigurationDoneArguments struct{}
type LoadedSourcesArguments struct{}
