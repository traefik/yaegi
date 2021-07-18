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

//go:generate go run ../cmd/json_schema --dap-mode --name dap --path types.go --patch patch.json --url https://microsoft.github.io/debug-adapter-protocol/debugAdapterProtocol.json

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w}
}

func (c *Encoder) Encode(msg IProtocolMessage) error {
	msg.prepareForEncoding()

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

type IProtocolMessage interface{ prepareForEncoding() }

func (m *ProtocolMessage) prepareForEncoding() {}

func (m *Request) prepareForEncoding()  { m.Type = ProtocolMessageType_Request }
func (m *Response) prepareForEncoding() { m.Type = ProtocolMessageType_Response }
func (m *Event) prepareForEncoding()    { m.Type = ProtocolMessageType_Event }

func (r *Request) UnmarshalJSON(b []byte) error {
	var x struct{ Command string }
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	switch x.Command {
	case "cancel":
		r.Arguments = new(CancelArguments)
	case "runInTerminal":
		r.Arguments = new(RunInTerminalRequestArguments)
	case "initialize":
		r.Arguments = new(InitializeRequestArguments)
	case "configurationDone":
		r.Arguments = new(ConfigurationDoneArguments)
	case "launch":
		r.Arguments = new(LaunchRequestArguments)
	case "attach":
		r.Arguments = new(AttachRequestArguments)
	case "restart":
		r.Arguments = new(RestartArguments)
	case "disconnect":
		r.Arguments = new(DisconnectArguments)
	case "terminate":
		r.Arguments = new(TerminateArguments)
	case "breakpointLocations":
		r.Arguments = new(BreakpointLocationsArguments)
	case "setBreakpoints":
		r.Arguments = new(SetBreakpointsArguments)
	case "setFunctionBreakpoints":
		r.Arguments = new(SetFunctionBreakpointsArguments)
	case "setExceptionBreakpoints":
		r.Arguments = new(SetExceptionBreakpointsArguments)
	case "dataBreakpointInfo":
		r.Arguments = new(DataBreakpointInfoArguments)
	case "setDataBreakpoints":
		r.Arguments = new(SetDataBreakpointsArguments)
	case "setInstructionBreakpoints":
		r.Arguments = new(SetInstructionBreakpointsArguments)
	case "continue":
		r.Arguments = new(ContinueArguments)
	case "next":
		r.Arguments = new(NextArguments)
	case "stepIn":
		r.Arguments = new(StepInArguments)
	case "stepOut":
		r.Arguments = new(StepOutArguments)
	case "stepBack":
		r.Arguments = new(StepBackArguments)
	case "reverseContinue":
		r.Arguments = new(ReverseContinueArguments)
	case "restartFrame":
		r.Arguments = new(RestartFrameArguments)
	case "goto":
		r.Arguments = new(GotoArguments)
	case "pause":
		r.Arguments = new(PauseArguments)
	case "stackTrace":
		r.Arguments = new(StackTraceArguments)
	case "scopes":
		r.Arguments = new(ScopesArguments)
	case "variables":
		r.Arguments = new(VariablesArguments)
	case "setVariable":
		r.Arguments = new(SetVariableArguments)
	case "source":
		r.Arguments = new(SourceArguments)
	case "threads":
		r.Arguments = new(ThreadsArguments)
	case "terminateThreads":
		r.Arguments = new(TerminateThreadsArguments)
	case "modules":
		r.Arguments = new(ModulesArguments)
	case "loadedSources":
		r.Arguments = new(LoadedSourcesArguments)
	case "evaluate":
		r.Arguments = new(EvaluateArguments)
	case "setExpression":
		r.Arguments = new(SetExpressionArguments)
	case "stepInTargets":
		r.Arguments = new(StepInTargetsArguments)
	case "gotoTargets":
		r.Arguments = new(GotoTargetsArguments)
	case "completions":
		r.Arguments = new(CompletionsArguments)
	case "exceptionInfo":
		r.Arguments = new(ExceptionInfoArguments)
	case "readMemory":
		r.Arguments = new(ReadMemoryArguments)
	case "disassemble":
		r.Arguments = new(DisassembleArguments)
	default:
		return fmt.Errorf("unrecognized command %q", r.Type)
	}

	type T Request
	return json.Unmarshal(b, (*T)(r))
}

func (r *Response) UnmarshalJSON(b []byte) error {
	var x struct{ Command string }
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	switch x.Command {
	case "initialize":
		r.Body = new(Capabilities)
	case "error":
		r.Body = new(ErrorResponseBody)
	case "cancel":
		r.Body = new(CancelResponseBody)
	case "runInTerminal":
		r.Body = new(RunInTerminalResponseBody)
	case "configurationDone":
		r.Body = new(ConfigurationDoneResponseBody)
	case "launch":
		r.Body = new(LaunchResponseBody)
	case "attach":
		r.Body = new(AttachResponseBody)
	case "restart":
		r.Body = new(RestartResponseBody)
	case "disconnect":
		r.Body = new(DisconnectResponseBody)
	case "terminate":
		r.Body = new(TerminateResponseBody)
	case "breakpointLocations":
		r.Body = new(BreakpointLocationsResponseBody)
	case "setBreakpoints":
		r.Body = new(SetBreakpointsResponseBody)
	case "setFunctionBreakpoints":
		r.Body = new(SetFunctionBreakpointsResponseBody)
	case "setExceptionBreakpoints":
		r.Body = new(SetExceptionBreakpointsResponseBody)
	case "dataBreakpointInfo":
		r.Body = new(DataBreakpointInfoResponseBody)
	case "setDataBreakpoints":
		r.Body = new(SetDataBreakpointsResponseBody)
	case "setInstructionBreakpoints":
		r.Body = new(SetInstructionBreakpointsResponseBody)
	case "continue":
		r.Body = new(ContinueResponseBody)
	case "next":
		r.Body = new(NextResponseBody)
	case "stepIn":
		r.Body = new(StepInResponseBody)
	case "stepOut":
		r.Body = new(StepOutResponseBody)
	case "stepBack":
		r.Body = new(StepBackResponseBody)
	case "reverseContinue":
		r.Body = new(ReverseContinueResponseBody)
	case "restartFrame":
		r.Body = new(RestartFrameResponseBody)
	case "goto":
		r.Body = new(GotoResponseBody)
	case "pause":
		r.Body = new(PauseResponseBody)
	case "stackTrace":
		r.Body = new(StackTraceResponseBody)
	case "scopes":
		r.Body = new(ScopesResponseBody)
	case "variables":
		r.Body = new(VariablesResponseBody)
	case "setVariable":
		r.Body = new(SetVariableResponseBody)
	case "source":
		r.Body = new(SourceResponseBody)
	case "threads":
		r.Body = new(ThreadsResponseBody)
	case "terminateThreads":
		r.Body = new(TerminateThreadsResponseBody)
	case "modules":
		r.Body = new(ModulesResponseBody)
	case "loadedSources":
		r.Body = new(LoadedSourcesResponseBody)
	case "evaluate":
		r.Body = new(EvaluateResponseBody)
	case "setExpression":
		r.Body = new(SetExpressionResponseBody)
	case "stepInTargets":
		r.Body = new(StepInTargetsResponseBody)
	case "gotoTargets":
		r.Body = new(GotoTargetsResponseBody)
	case "completions":
		r.Body = new(CompletionsResponseBody)
	case "exceptionInfo":
		r.Body = new(ExceptionInfoResponseBody)
	case "readMemory":
		r.Body = new(ReadMemoryResponseBody)
	case "disassemble":
		r.Body = new(DisassembleResponseBody)
	default:
		return fmt.Errorf("unrecognized command %q", r.Type)
	}

	type T Response
	return json.Unmarshal(b, (*T)(r))
}

func (e *Event) UnmarshalJSON(b []byte) error {
	var x struct{ Event string }
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	switch x.Event {
	case "initialized":
		e.Body = new(InitializedEventBody)
	case "stopped":
		e.Body = new(StoppedEventBody)
	case "continued":
		e.Body = new(ContinuedEventBody)
	case "exited":
		e.Body = new(ExitedEventBody)
	case "terminated":
		e.Body = new(TerminatedEventBody)
	case "thread":
		e.Body = new(ThreadEventBody)
	case "output":
		e.Body = new(OutputEventBody)
	case "breakpoint":
		e.Body = new(BreakpointEventBody)
	case "module":
		e.Body = new(ModuleEventBody)
	case "loadedSource":
		e.Body = new(LoadedSourceEventBody)
	case "process":
		e.Body = new(ProcessEventBody)
	case "capabilities":
		e.Body = new(CapabilitiesEventBody)
	case "progressStart":
		e.Body = new(ProgressStartEventBody)
	case "progressUpdate":
		e.Body = new(ProgressUpdateEventBody)
	case "progressEnd":
		e.Body = new(ProgressEndEventBody)
	case "invalidated":
		e.Body = new(InvalidatedEventBody)
	default:
		return fmt.Errorf("unrecognized command %q", e.Type)
	}

	type T Event
	return json.Unmarshal(b, (*T)(e))
}

type ConfigurationDoneArguments struct{}
type ThreadsArguments struct{}
type LoadedSourcesArguments struct{}

type CancelResponseBody struct{}
type ConfigurationDoneResponseBody struct{}
type LaunchResponseBody struct{}
type AttachResponseBody struct{}
type RestartResponseBody struct{}
type DisconnectResponseBody struct{}
type TerminateResponseBody struct{}
type SetExceptionBreakpointsResponseBody struct{}
type NextResponseBody struct{}
type StepInResponseBody struct{}
type StepOutResponseBody struct{}
type StepBackResponseBody struct{}
type ReverseContinueResponseBody struct{}
type RestartFrameResponseBody struct{}
type GotoResponseBody struct{}
type PauseResponseBody struct{}
type TerminateThreadsResponseBody struct{}

type InitializedEventBody struct{}
