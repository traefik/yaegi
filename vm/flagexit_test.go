package vm_test

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

const flagplug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/flagplug"

// flagServe loads flagplug (with its AOT copies or not) and calls path. It
// returns the response body and the panic that ServeHTTP raised.
func flagServe(t *testing.T, aot bool, path string) (body string, panicked any) {
	t.Helper()
	p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, flagplug)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := pluginhost.CreateConfig(p)
	h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "flag")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	defer func() {
		panicked = recover()
		body = w.Body.String()
	}()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return
}

// TestFlagExit checks that native code which calls os.Exit cannot end the
// host: flag.ExitOnError with a bad flag or -h (static call, method value,
// call through an interface, FlagSet.Init, the plugin's reflect, and
// text/template and html/template, which call Parse through the native
// reflect), and the Fatal method of a native *log.Logger. The plugin gets the panic of its own restricted
// os.Exit instead. Each case runs in a child process (this test binary),
// so that a real exit is seen: the child must end normally and print what
// it recovered.
func TestFlagExit(t *testing.T) {
	if c := os.Getenv("VM_FLAGEXIT_CHILD"); c != "" {
		mode, path, _ := strings.Cut(c, " ")
		body, r := flagServe(t, mode == "aot", path)
		fmt.Printf("body %q\npanic %v\nhost still alive\n", body, r)
		return
	}
	usage := "Usage of x:\n  -v\tverbose\n"
	bad := "flag provided but not defined: -bogus\n" + usage
	initBad := "flag provided but not defined: -bogus\nUsage of y:\n"
	tmplExit := "template: template: t:1:5: executing \"t\" at <.FS.Parse>: error calling Parse: os.Exit(2)\n"
	for _, c := range []struct {
		path, body, panic string
		modes             []string
	}{
		{"/exit", "", "os.Exit(2)", nil}, // the plugin's own os.Exit, for reference
		{"/bad", bad, "os.Exit(2)", nil},
		{"/help", usage, "os.Exit(0)", nil},
		{"/value", bad, "os.Exit(2)", nil},
		{"/iface", bad, "os.Exit(2)", nil},
		{"/init", initBad, "os.Exit(2)", nil},
		// A template that calls Parse: the AOT copy of flag panics with the
		// plugin's os.Exit, and the template returns that panic as an
		// error. Default mode runs the standard library as in AOT mode for
		// a plugin that imports a template package (stdAOTModes), when the
		// host has the copies, as here. (With the native packages, the
		// native flag set has flag.ContinueOnError, so Parse returns the
		// error, and text/template prints it.)
		{"/template", bad + tmplExit, "<nil>", nil},
		{"/htmltemplate", bad + tmplExit, "<nil>", nil},
		// The plugin's own reflect gets the replaced methods.
		{"/reflect", bad, "os.Exit(2)", nil},
		{"/reflectinit", initBad, "os.Exit(2)", nil},
		{"/reflectboth", initBad, "os.Exit(2)", nil},
		{"/reflecttype", initBad, "os.Exit(2)", nil},
		{"/reflectindex", initBad, "os.Exit(2)", nil},
		{"/reflectembed", initBad, "os.Exit(2)", nil},
		{"/errorhandling", "true true true\n", "<nil>", nil},
		// text/template does not call methods with no result.
		{"/templatelogger", "template: template: t:1:4: executing \"t\" at <.L.Fatal>: function Fatal has 0 return values; should be 1 or 2\n", "<nil>", nil},
		{"/reflectlogger", "", "logger fatal", []string{"default"}},
		// In AOT mode, the *log.Logger of the AOT copy of slog panics with a
		// nil dereference at the first call, also before this fix.
		{"/logger", "", "logger fatal", []string{"default"}},
	} {
		if c.modes == nil {
			c.modes = []string{"default", "aot"}
		}
		for _, mode := range c.modes {
			cmd := exec.Command(os.Args[0], "-test.run=^TestFlagExit$", "-test.v")
			cmd.Env = append(os.Environ(), "VM_FLAGEXIT_CHILD="+mode+" "+c.path)
			out, err := cmd.CombinedOutput()
			want := fmt.Sprintf("body %q\npanic %v\nhost still alive\n", c.body, c.panic)
			if err != nil || !strings.Contains(string(out), want) {
				t.Errorf("%s %s: child process: %v, output:\n%s\nwant:\n%s", mode, c.path, err, out, want)
			}
		}
	}
}

// TestFlagCommandLine checks that a plugin has its own flag.CommandLine,
// flag.Usage and os.Args: flag.Bool, flag.Parse and the assignments of the
// plugin do not change those of the host, in default and AOT modes. A
// flag.Parse that fails ends with the restricted os.Exit(2).
func TestFlagCommandLine(t *testing.T) {
	args := append([]string(nil), os.Args...)
	usage := reflect.ValueOf(flag.Usage).Pointer()
	for _, aot := range []bool{false, true} {
		body, r := flagServe(t, aot, "/args")
		if body != strings.Join(args, " ") || r != nil {
			t.Errorf("AOT %v: /args: %q, panic %v; want the host's arguments %q", aot, body, r, args)
		}
		// A Parse that succeeds keeps flag.ExitOnError.
		if body, r = flagServe(t, aot, "/ok"); body != "<nil> true [a] true\n" || r != nil {
			t.Errorf("AOT %v: /ok: %q, panic %v", aot, body, r)
		}
		body, r = flagServe(t, aot, "/cmdline")
		// The plugin's flag.CommandLine is named after the host's os.Args[0]
		// (package flag does so in its init), before the plugin changes its
		// own os.Args.
		want := fmt.Sprintf("true true [rest] %s\n", args[0]) +
			"flag provided but not defined: -nope\nplugin usage\n"
		if body != want || fmt.Sprint(r) != "os.Exit(2)" {
			t.Errorf("AOT %v: /cmdline:\n%q, panic %v\nwant\n%q, panic os.Exit(2)", aot, body, r, want)
		}
		if flag.Lookup("plugonly") != nil {
			t.Errorf("AOT %v: the plugin defined a flag of the host", aot)
		}
		if !reflect.DeepEqual(os.Args, args) || reflect.ValueOf(flag.Usage).Pointer() != usage {
			t.Errorf("AOT %v: the plugin changed the host's os.Args or flag.Usage: %q", aot, os.Args)
		}
	}
}

// TestFlagCommandLinePerPlugin loads two instances of plugins that define a
// flag on flag.CommandLine when they are initialized: in default mode, flag
// is native and each machine has its own CommandLine (exit.go), also when
// another import (text/template) makes the standard library run as AOT
// copies, and in AOT mode. The AOT copy of flag has one CommandLine for
// the whole process: when flag ran as a copy, the second instance panicked
// with "flag redefined: dup".
func TestFlagCommandLinePerPlugin(t *testing.T) {
	for _, aot := range []bool{false, true} {
		for _, plug := range []string{"flagdupplug", "flagduptmplplug"} {
			for i := range 2 {
				p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/"+plug)
				if err != nil {
					t.Fatalf("AOT %v: %s: instance %d: %v", aot, plug, i, err)
				}
				cfg, _ := pluginhost.CreateConfig(p)
				h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "flagdup")
				if err != nil {
					t.Fatalf("AOT %v: %s: instance %d: %v", aot, plug, i, err)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
				if got := w.Body.String(); got != "ok false\n" {
					t.Errorf("AOT %v: %s: instance %d: %q, want \"ok false\\n\"", aot, plug, i, got)
				}
			}
		}
	}
	if flag.Lookup("dup") != nil {
		t.Error("the plugin defined a flag of the host")
	}
}
