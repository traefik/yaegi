//go:build unix

package vm

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestRefusedMsgMethods checks that a restricted plugin cannot send a
// socket control message (msgplug): the WriteMsg methods are refused when
// named, through an interface and through reflect, and the ReadMsg methods
// deliver no control message. Unrestricted, the methods run as in Go.
func TestRefusedMsgMethods(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "s")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	// The server echoes one message with a control message that passes a
	// descriptor (a copy of its stdout).
	go func() {
		for {
			c, err := l.AcceptUnix()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				b := make([]byte, 16)
				n, _ := c.Read(b)
				oob := syscall.UnixRights(int(os.Stdout.Fd()))
				c.WriteMsgUnix(b[:n], oob, nil)
			}()
		}
	}()
	for _, opts := range []Options{{}, {AOT: true}} {
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/msgplug")
		if err != nil {
			t.Fatal(err)
		}
		for _, how := range []string{"interface", "reflect"} {
			res, err := p.Call(context.Background(), "Write", path, how)
			if err != nil {
				t.Fatal(err)
			}
			msg, _ := res[0].Interface()
			if !strings.Contains(fmt.Sprint(msg), "WriteMsgUnix is not allowed in a plugin") {
				t.Errorf("aot %v, %s: got %q, want a refusal", opts.AOT, how, msg)
			}
		}
		res, err := p.Call(context.Background(), "Read", path)
		if err != nil {
			t.Fatal(err)
		}
		oobn, _ := res[0].Interface()
		if rerr, _ := res[1].Interface(); rerr != nil {
			t.Errorf("aot %v, Read: %v", opts.AOT, rerr)
		} else if oobn != 0 {
			t.Errorf("aot %v, Read: %v oob bytes delivered, want 0", opts.AOT, oobn)
		}
	}
	p, err := Load(Options{Dir: "..", Unrestricted: true}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/msgplug")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Call(context.Background(), "Write", path, "interface")
	if err != nil {
		t.Fatal(err)
	}
	// The oob of the plugin is not a valid control message: the kernel
	// refuses it, which shows that the native method ran.
	if msg, _ := res[0].Interface(); !strings.Contains(fmt.Sprint(msg), "sendmsg") {
		t.Errorf("unrestricted: got %q, want the error of sendmsg", msg)
	}
	res, err = p.Call(context.Background(), "Read", path)
	if err != nil {
		t.Fatal(err)
	}
	if oobn, _ := res[0].Interface(); oobn == 0 {
		t.Error("unrestricted: no oob bytes delivered")
	}
}

// shortTempDir returns a new directory for a unix socket. The path of a
// socket has at most 104 bytes on macOS and 108 on Linux: t.TempDir adds
// the name of the test and two levels to TMPDIR, and is too long when
// TMPDIR is long (macOS, or a CI workspace). Then it uses /tmp.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "m")
	if err == nil && len(dir) > 90 {
		os.Remove(dir)
		dir, err = os.MkdirTemp("/tmp", "m")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
