// Code generated by 'yaegi extract syscall'. DO NOT EDIT.

// +build go1.16

package unrestricted

import (
	"reflect"
	"syscall"
)

func init() {
	Symbols["syscall"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Exec":         reflect.ValueOf(syscall.Exec),
		"Exit":         reflect.ValueOf(syscall.Exit),
		"ForkExec":     reflect.ValueOf(syscall.ForkExec),
		"Kill":         reflect.ValueOf(syscall.Kill),
		"RawSyscall":   reflect.ValueOf(syscall.RawSyscall),
		"Shutdown":     reflect.ValueOf(syscall.Shutdown),
		"StartProcess": reflect.ValueOf(syscall.StartProcess),
		"Syscall":      reflect.ValueOf(syscall.Syscall),
	}
}
