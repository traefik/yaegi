// Package execmodeplug uses os/exec: loaded with Options.Exec (ExecMock or
// ExecReal).
package execmodeplug

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"text/template"
)

// GoOS runs "go env GOOS" with exec.Command.
func GoOS() string {
	out, err := exec.Command("go", "env", "GOOS").Output()
	if err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

// Write runs a shell that writes file, with a Cmd built by hand.
func Write(file string) string {
	c := &exec.Cmd{Path: "/bin/sh", Args: []string{"sh", "-c", "echo x > " + file}}
	if err := c.Run(); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

// Errors reports the errors of LookPath, os.StartProcess, CombinedOutput
// and Start, and whether they are exec.ErrNotFound.
func Errors() string {
	var out []string
	_, err := exec.LookPath("go")
	out = append(out, errState(err))
	_, err = os.StartProcess("/bin/sh", []string{"sh", "-c", "true"}, &os.ProcAttr{})
	out = append(out, errState(err))
	_, err = exec.Command("go", "version").CombinedOutput()
	out = append(out, errState(err))
	c := exec.Command("go", "version")
	c.Err = nil // a plugin cannot clear the refusal
	out = append(out, errState(c.Start()))
	return strings.Join(out, " ")
}

func errState(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, exec.ErrNotFound):
		return "notfound"
	}
	return "other"
}

// Dyn starts a Cmd by the other ways a program can reach a method: through
// an interface, a method value, a method expression, reflect, and a
// function of the FuncMap of a native template. Each one reports its error.
func Dyn() string {
	var out []string
	cmd := func() *exec.Cmd { return exec.Command("go", "version") } // a Cmd runs once
	var i interface{ Start() error } = cmd()
	out = append(out, errState(i.Start()))
	f := cmd().Run
	out = append(out, errState(f()))
	g := (*exec.Cmd).Run
	out = append(out, errState(g(cmd())))
	rv := reflect.ValueOf(cmd()).MethodByName("Start").Call(nil)
	err, _ := rv[0].Interface().(error)
	out = append(out, errState(err))
	rv = reflect.ValueOf(cmd()).MethodByName("CombinedOutput").Call(nil)
	err, _ = rv[1].Interface().(error)
	out = append(out, errState(err))
	t := template.Must(template.New("t").Funcs(template.FuncMap{"run": cmd().Run}).Parse("{{run}}"))
	var b bytes.Buffer
	if err := t.Execute(&b, nil); err != nil {
		out = append(out, errState(err))
	} else {
		out = append(out, "ran:"+b.String())
	}
	return strings.Join(out, " ")
}

// Environ reports the environment of a Cmd made by exec.Command, of one
// built by hand, and of one seen by a native template ({{.Environ}}): "env"
// when it is the environment the plugin sees (os.Environ), else the whole
// environment.
func Environ() string {
	want := strings.Join(os.Environ(), ",")
	same := func(env []string) string {
		// On Windows, Cmd.Environ adds SYSTEMROOT when the environment
		// does not have it, as in Go.
		if !strings.Contains(want, "SYSTEMROOT=") {
			var kept []string
			for _, kv := range env {
				if !strings.HasPrefix(kv, "SYSTEMROOT=") {
					kept = append(kept, kv)
				}
			}
			env = kept
		}
		if strings.Join(env, ",") == want {
			return "env"
		}
		return strings.Join(env, ",")
	}
	out := []string{same(exec.Command("go", "version").Environ()), same((&exec.Cmd{Path: "/bin/sh"}).Environ())}
	// one entry per line: an entry may hold spaces
	t := template.Must(template.New("t").Parse("{{range .Environ}}{{.}}\n{{end}}"))
	var b bytes.Buffer
	if err := t.Execute(&b, &exec.Cmd{Path: "/bin/sh"}); err != nil {
		out = append(out, "error: "+err.Error())
	} else {
		out = append(out, same(strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")))
	}
	return strings.Join(out, " ")
}

// Template calls the Run method of a Cmd from a native template.
func Template() string {
	t := template.Must(template.New("t").Parse("{{.Run}}"))
	var b bytes.Buffer
	if err := t.Execute(&b, exec.Command("go", "version")); err != nil {
		return "error: " + err.Error()
	}
	return b.String()
}
