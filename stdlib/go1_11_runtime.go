// +build go1.11, !go1.12

package stdlib

// Code generated by 'goexports runtime'. DO NOT EDIT.

import (
	"reflect"
	"runtime"
)

func init() {
	Value["runtime"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"BlockProfile":            reflect.ValueOf(runtime.BlockProfile),
		"Breakpoint":              reflect.ValueOf(runtime.Breakpoint),
		"CPUProfile":              reflect.ValueOf(runtime.CPUProfile),
		"Caller":                  reflect.ValueOf(runtime.Caller),
		"Callers":                 reflect.ValueOf(runtime.Callers),
		"CallersFrames":           reflect.ValueOf(runtime.CallersFrames),
		"Compiler":                reflect.ValueOf(runtime.Compiler),
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
	}
}
