// Package unrestricted provides the original version of standard library symbols which may cause the interpreter process to exit.
package unrestricted

import (
	"log"
	"os"
	"os/exec"
	"reflect"
)

// Symbols stores the map of syscall package symbols.
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["os/os"] = map[string]reflect.Value{
		"Exit":        reflect.ValueOf(os.Exit),
		"FindProcess": reflect.ValueOf(os.FindProcess),
	}

	Symbols["os/exec/exec"] = map[string]reflect.Value{
		"Command":        reflect.ValueOf(exec.Command),
		"CommandContext": reflect.ValueOf(exec.CommandContext),
		"ErrNotFound":    reflect.ValueOf(&exec.ErrNotFound).Elem(),
		"LookPath":       reflect.ValueOf(exec.LookPath),
		"Cmd":            reflect.ValueOf((*exec.Cmd)(nil)),
		"Error":          reflect.ValueOf((*exec.Error)(nil)),
		"ExitError":      reflect.ValueOf((*exec.ExitError)(nil)),
	}

	Symbols["log/log"] = map[string]reflect.Value{
		"Fatal":   reflect.ValueOf(log.Fatal),
		"Fatalf":  reflect.ValueOf(log.Fatalf),
		"Fatalln": reflect.ValueOf(log.Fatalln),
		"New":     reflect.ValueOf(log.New),
		"Logger":  reflect.ValueOf((*log.Logger)(nil)),
	}

	Symbols["github.com/traefik/yaegi/stdlib/unrestricted/unrestricted"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

//go:generate ../../internal/cmd/extract/extract -include=^Exec,Exit,ForkExec,Kill,Ptrace,Reboot,Shutdown,StartProcess,Syscall syscall
