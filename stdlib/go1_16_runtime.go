// Code generated by 'yaegi extract runtime'. DO NOT EDIT.

// +build go1.16

package stdlib

import (
	"go/constant"
	"go/token"
	"reflect"
	"runtime"
)

func init() {
	Symbols["runtime"] = map[string]reflect.Value{
		// default package name identifier
		".name": reflect.ValueOf("runtime"),

		// function, constant and variable definitions
		"BlockProfile":            reflect.ValueOf(runtime.BlockProfile),
		"Breakpoint":              reflect.ValueOf(runtime.Breakpoint),
		"CPUProfile":              reflect.ValueOf(runtime.CPUProfile),
		"Caller":                  reflect.ValueOf(runtime.Caller),
		"Callers":                 reflect.ValueOf(runtime.Callers),
		"CallersFrames":           reflect.ValueOf(runtime.CallersFrames),
		"Compiler":                reflect.ValueOf(constant.MakeFromLiteral("\"gc\"", token.STRING, 0)),
		"FuncForPC":               reflect.ValueOf(runtime.FuncForPC),
		"GC":                      reflect.ValueOf(runtime.GC),
		"GOARCH":                  reflect.ValueOf(runtime.GOARCH),
		"GOMAXPROCS":              reflect.ValueOf(runtime.GOMAXPROCS),
		"GOOS":                    reflect.ValueOf(runtime.GOOS),
		"GOROOT":                  reflect.ValueOf(runtime.GOROOT),
		"Goexit":                  reflect.ValueOf(runtime.Goexit),
		"GoroutineProfile":        reflect.ValueOf(runtime.GoroutineProfile),
		"Gosched":                 reflect.ValueOf(runtime.Gosched),
		"KeepAlive":               reflect.ValueOf(runtime.KeepAlive),
		"LockOSThread":            reflect.ValueOf(runtime.LockOSThread),
		"MemProfile":              reflect.ValueOf(runtime.MemProfile),
		"MemProfileRate":          reflect.ValueOf(&runtime.MemProfileRate).Elem(),
		"MutexProfile":            reflect.ValueOf(runtime.MutexProfile),
		"NumCPU":                  reflect.ValueOf(runtime.NumCPU),
		"NumCgoCall":              reflect.ValueOf(runtime.NumCgoCall),
		"NumGoroutine":            reflect.ValueOf(runtime.NumGoroutine),
		"ReadMemStats":            reflect.ValueOf(runtime.ReadMemStats),
		"ReadTrace":               reflect.ValueOf(runtime.ReadTrace),
		"SetBlockProfileRate":     reflect.ValueOf(runtime.SetBlockProfileRate),
		"SetCPUProfileRate":       reflect.ValueOf(runtime.SetCPUProfileRate),
		"SetCgoTraceback":         reflect.ValueOf(runtime.SetCgoTraceback),
		"SetFinalizer":            reflect.ValueOf(runtime.SetFinalizer),
		"SetMutexProfileFraction": reflect.ValueOf(runtime.SetMutexProfileFraction),
		"Stack":                   reflect.ValueOf(runtime.Stack),
		"StartTrace":              reflect.ValueOf(runtime.StartTrace),
		"StopTrace":               reflect.ValueOf(runtime.StopTrace),
		"ThreadCreateProfile":     reflect.ValueOf(runtime.ThreadCreateProfile),
		"UnlockOSThread":          reflect.ValueOf(runtime.UnlockOSThread),
		"Version":                 reflect.ValueOf(runtime.Version),

		// type definitions
		"BlockProfileRecord": reflect.ValueOf((*runtime.BlockProfileRecord)(nil)),
		"Error":              reflect.ValueOf((*runtime.Error)(nil)),
		"Frame":              reflect.ValueOf((*runtime.Frame)(nil)),
		"Frames":             reflect.ValueOf((*runtime.Frames)(nil)),
		"Func":               reflect.ValueOf((*runtime.Func)(nil)),
		"MemProfileRecord":   reflect.ValueOf((*runtime.MemProfileRecord)(nil)),
		"MemStats":           reflect.ValueOf((*runtime.MemStats)(nil)),
		"StackRecord":        reflect.ValueOf((*runtime.StackRecord)(nil)),
		"TypeAssertionError": reflect.ValueOf((*runtime.TypeAssertionError)(nil)),

		// interface wrapper definitions
		"_Error": reflect.ValueOf((*_runtime_Error)(nil)),
	}
}

// _runtime_Error is an interface wrapper for Error type
type _runtime_Error struct {
	WError        func() string
	WRuntimeError func()
}

func (W _runtime_Error) Error() string { return W.WError() }
func (W _runtime_Error) RuntimeError() { W.WRuntimeError() }
