package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHostileCorpus runs each plugin of the hostile corpus (hostile.go) in a
// child process, in the default mode and in AOT mode, and checks that each
// guard rail held and that the host process is still alive.
func TestHostileCorpus(t *testing.T) {
	for _, mode := range []string{"vm", "aot"} {
		t.Run(mode, func(t *testing.T) {
			var out strings.Builder
			ok := runHostileChildren(&out, hostileNames(), func(string) *exec.Cmd {
				c := exec.Command(os.Args[0], "-test.run=^TestHostileChild$", "-test.v")
				c.Env = append(os.Environ(), "VMHOST_HOSTILE_MODE="+mode)
				return c
			})
			t.Log("\n" + out.String())
			if !ok {
				t.Fatal("a guard rail did not hold")
			}
		})
	}
}

// TestHostileChild is the child process of TestHostileCorpus: it runs the
// case named by VMHOST_HOSTILE, and is skipped in a normal run.
func TestHostileChild(t *testing.T) {
	name := os.Getenv(hostileEnv)
	if name == "" {
		t.Skip("the child process of TestHostileCorpus")
	}
	if !runHostileCase(os.Stdout, "../..", os.Getenv("VMHOST_HOSTILE_MODE") == "aot", name) {
		t.Fail()
	}
}
