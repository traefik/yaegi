// Package sandboxplug exercises the capabilities of a yaegi v2 Sandbox: its
// environment, the granted and denied filesystem, and a denied network dial.
package sandboxplug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// result turns an error into "ok" or "denied" (the sandbox error) or the
// raw error, so the test can assert a stable string.
func result(err error) string {
	switch {
	case err == nil:
		return "ok"
	case strings.Contains(err.Error(), "denied by the plugin sandbox"):
		return "denied"
	default:
		return "err"
	}
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in := os.Getenv("YAEGI_IN")       // granted read file
		out := os.Getenv("YAEGI_OUT")     // granted write file
		denied := os.Getenv("YAEGI_DENY") // ungranted path

		readIn, errIn := os.ReadFile(in)
		_, errDenyRead := os.ReadFile(denied)
		errWrite := os.WriteFile(out, []byte("x"), 0o644)
		errDenyWrite := os.WriteFile(denied, []byte("x"), 0o644)
		_, errDial := net.Dial("tcp", "10.0.0.1:9")

		fmt.Fprintf(w, "env=%q in=%s(%s) denyread=%s write=%s denywrite=%s dial=%s",
			os.Getenv("YAEGI_SB"), result(errIn), strings.TrimSpace(string(readIn)),
			result(errDenyRead), result(errWrite), result(errDenyWrite), result(errDial))
	}), nil
}
