// Code generated by 'yaegi extract syscall'. DO NOT EDIT.

// +build go1.16

package unrestricted

import (
	"reflect"
	"syscall"
)

func init() {
	Symbols["syscall"] = map[string]reflect.Value{
		// default package name identifier
		".name": reflect.ValueOf("syscall"),

		// function, constant and variable definitions
		"Exec":               reflect.ValueOf(syscall.Exec),
		"Exit":               reflect.ValueOf(syscall.Exit),
		"ExitProcess":        reflect.ValueOf(syscall.ExitProcess),
		"GetExitCodeProcess": reflect.ValueOf(syscall.GetExitCodeProcess),
		"Shutdown":           reflect.ValueOf(syscall.Shutdown),
		"StartProcess":       reflect.ValueOf(syscall.StartProcess),
		"Syscall":            reflect.ValueOf(syscall.Syscall),
		"Syscall12":          reflect.ValueOf(syscall.Syscall12),
		"Syscall15":          reflect.ValueOf(syscall.Syscall15),
		"Syscall18":          reflect.ValueOf(syscall.Syscall18),
		"Syscall6":           reflect.ValueOf(syscall.Syscall6),
		"Syscall9":           reflect.ValueOf(syscall.Syscall9),
	}
}
