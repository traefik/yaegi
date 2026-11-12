package vm

import "testing"

// TestNewFileRefused checks that os.NewFile panics with a *RefusedError in
// a restricted plugin (newfile.go): with the number of the host stdout,
// with the number of a pipe that the plugin opened, and through a function
// value, with and without useUnsafe and with a Sandbox, in both modes. The
// plugin loads: only the call is refused. An unrestricted plugin keeps the
// os.NewFile of Go (nil for an invalid number). It does not call it with a
// real number: the new *os.File would close that descriptor of the test
// process when the garbage collector frees it.
func TestNewFileRefused(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/newfileplug"
	const refused = "os.NewFile (a file of any descriptor number) is not allowed in a plugin"
	for _, opts := range []Options{
		{Dir: ".."},
		{Dir: "..", AOT: true},
		{Dir: "..", UseUnsafe: true},
		{Dir: "..", Sandbox: &Sandbox{}},
	} {
		for _, path := range []string{"/stdout", "/own", "/value", "/invalid"} {
			if got := serveOnce(t, opts, pkg, path); got != refused {
				t.Errorf("useUnsafe %v, AOT %v, sandbox %v, %s: %q, want %q", opts.UseUnsafe, opts.AOT, opts.Sandbox != nil, path, got, refused)
			}
		}
	}
	for _, aot := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: "..", Unrestricted: true, AOT: aot}, pkg, "/invalid"); got != "true" {
			t.Errorf("unrestricted, AOT %v: %q, want true", aot, got)
		}
	}
}
