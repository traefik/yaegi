package dap

type RequestArguments interface{ requestType() string }
type ResponseBody interface{ responseType() string }
type EventBody interface{ eventType() string }

func (*CancelArguments) requestType() string                    { return "cancel" }
func (*RunInTerminalRequestArguments) requestType() string      { return "runInTerminal" }
func (*InitializeRequestArguments) requestType() string         { return "initialize" }
func (*ConfigurationDoneArguments) requestType() string         { return "configurationDone" }
func (*LaunchRequestArguments) requestType() string             { return "launch" }
func (*AttachRequestArguments) requestType() string             { return "attach" }
func (*RestartArguments) requestType() string                   { return "restart" }
func (*DisconnectArguments) requestType() string                { return "disconnect" }
func (*TerminateArguments) requestType() string                 { return "terminate" }
func (*BreakpointLocationsArguments) requestType() string       { return "breakpointLocations" }
func (*SetBreakpointsArguments) requestType() string            { return "setBreakpoints" }
func (*SetFunctionBreakpointsArguments) requestType() string    { return "setFunctionBreakpoints" }
func (*SetExceptionBreakpointsArguments) requestType() string   { return "setExceptionBreakpoints" }
func (*DataBreakpointInfoArguments) requestType() string        { return "dataBreakpointInfo" }
func (*SetDataBreakpointsArguments) requestType() string        { return "setDataBreakpoints" }
func (*SetInstructionBreakpointsArguments) requestType() string { return "setInstructionBreakpoints" }
func (*ContinueArguments) requestType() string                  { return "continue" }
func (*NextArguments) requestType() string                      { return "next" }
func (*StepInArguments) requestType() string                    { return "stepIn" }
func (*StepOutArguments) requestType() string                   { return "stepOut" }
func (*StepBackArguments) requestType() string                  { return "stepBack" }
func (*ReverseContinueArguments) requestType() string           { return "reverseContinue" }
func (*RestartFrameArguments) requestType() string              { return "restartFrame" }
func (*GotoArguments) requestType() string                      { return "goto" }
func (*PauseArguments) requestType() string                     { return "pause" }
func (*StackTraceArguments) requestType() string                { return "stackTrace" }
func (*ScopesArguments) requestType() string                    { return "scopes" }
func (*VariablesArguments) requestType() string                 { return "variables" }
func (*SetVariableArguments) requestType() string               { return "setVariable" }
func (*SourceArguments) requestType() string                    { return "source" }
func (*ThreadsArguments) requestType() string                   { return "threads" }
func (*TerminateThreadsArguments) requestType() string          { return "terminateThreads" }
func (*ModulesArguments) requestType() string                   { return "modules" }
func (*LoadedSourcesArguments) requestType() string             { return "loadedSources" }
func (*EvaluateArguments) requestType() string                  { return "evaluate" }
func (*SetExpressionArguments) requestType() string             { return "setExpression" }
func (*StepInTargetsArguments) requestType() string             { return "stepInTargets" }
func (*GotoTargetsArguments) requestType() string               { return "gotoTargets" }
func (*CompletionsArguments) requestType() string               { return "completions" }
func (*ExceptionInfoArguments) requestType() string             { return "exceptionInfo" }
func (*ReadMemoryArguments) requestType() string                { return "readMemory" }
func (*DisassembleArguments) requestType() string               { return "disassemble" }

func (*Capabilities) responseType() string                        { return "initialize" }
func (*ErrorResponseBody) responseType() string                   { return "error" }
func (*CancelResponseBody) responseType() string                  { return "cancel" }
func (*RunInTerminalResponseBody) responseType() string           { return "runInTerminal" }
func (*ConfigurationDoneResponseBody) responseType() string       { return "configurationDone" }
func (*LaunchResponseBody) responseType() string                  { return "launch" }
func (*AttachResponseBody) responseType() string                  { return "attach" }
func (*RestartResponseBody) responseType() string                 { return "restart" }
func (*DisconnectResponseBody) responseType() string              { return "disconnect" }
func (*TerminateResponseBody) responseType() string               { return "terminate" }
func (*BreakpointLocationsResponseBody) responseType() string     { return "breakpointLocations" }
func (*SetBreakpointsResponseBody) responseType() string          { return "setBreakpoints" }
func (*SetFunctionBreakpointsResponseBody) responseType() string  { return "setFunctionBreakpoints" }
func (*SetExceptionBreakpointsResponseBody) responseType() string { return "setExceptionBreakpoints" }
func (*DataBreakpointInfoResponseBody) responseType() string      { return "dataBreakpointInfo" }
func (*SetDataBreakpointsResponseBody) responseType() string      { return "setDataBreakpoints" }
func (*ContinueResponseBody) responseType() string                { return "continue" }
func (*NextResponseBody) responseType() string                    { return "next" }
func (*StepInResponseBody) responseType() string                  { return "stepIn" }
func (*StepOutResponseBody) responseType() string                 { return "stepOut" }
func (*StepBackResponseBody) responseType() string                { return "stepBack" }
func (*ReverseContinueResponseBody) responseType() string         { return "reverseContinue" }
func (*RestartFrameResponseBody) responseType() string            { return "restartFrame" }
func (*GotoResponseBody) responseType() string                    { return "goto" }
func (*PauseResponseBody) responseType() string                   { return "pause" }
func (*StackTraceResponseBody) responseType() string              { return "stackTrace" }
func (*ScopesResponseBody) responseType() string                  { return "scopes" }
func (*VariablesResponseBody) responseType() string               { return "variables" }
func (*SetVariableResponseBody) responseType() string             { return "setVariable" }
func (*SourceResponseBody) responseType() string                  { return "source" }
func (*ThreadsResponseBody) responseType() string                 { return "threads" }
func (*TerminateThreadsResponseBody) responseType() string        { return "terminateThreads" }
func (*ModulesResponseBody) responseType() string                 { return "modules" }
func (*LoadedSourcesResponseBody) responseType() string           { return "loadedSources" }
func (*EvaluateResponseBody) responseType() string                { return "evaluate" }
func (*SetExpressionResponseBody) responseType() string           { return "setExpression" }
func (*StepInTargetsResponseBody) responseType() string           { return "stepInTargets" }
func (*GotoTargetsResponseBody) responseType() string             { return "gotoTargets" }
func (*CompletionsResponseBody) responseType() string             { return "completions" }
func (*ExceptionInfoResponseBody) responseType() string           { return "exceptionInfo" }
func (*ReadMemoryResponseBody) responseType() string              { return "readMemory" }
func (*DisassembleResponseBody) responseType() string             { return "disassemble" }

func (*SetInstructionBreakpointsResponseBody) responseType() string {
	return "setInstructionBreakpoints"
}

func (*InitializedEventBody) eventType() string    { return "initialized" }
func (*StoppedEventBody) eventType() string        { return "stopped" }
func (*ContinuedEventBody) eventType() string      { return "continued" }
func (*ExitedEventBody) eventType() string         { return "exited" }
func (*TerminatedEventBody) eventType() string     { return "terminated" }
func (*ThreadEventBody) eventType() string         { return "thread" }
func (*OutputEventBody) eventType() string         { return "output" }
func (*BreakpointEventBody) eventType() string     { return "breakpoint" }
func (*ModuleEventBody) eventType() string         { return "module" }
func (*LoadedSourceEventBody) eventType() string   { return "loadedSource" }
func (*ProcessEventBody) eventType() string        { return "process" }
func (*CapabilitiesEventBody) eventType() string   { return "capabilities" }
func (*ProgressStartEventBody) eventType() string  { return "progressStart" }
func (*ProgressUpdateEventBody) eventType() string { return "progressUpdate" }
func (*ProgressEndEventBody) eventType() string    { return "progressEnd" }
func (*InvalidatedEventBody) eventType() string    { return "invalidated" }
