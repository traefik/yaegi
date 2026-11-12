package vm

import (
	"os/exec"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestYaegiTagsPlugin checks Options.YaegiTags with both loaders: with it,
// Load uses the build tags of a "// yaegi:tags" line of the plugin package,
// as yaegi does, in the plugin package and in its vendored packages (a
// plugin that needs a tag, as Coraza needs "tinygo", can set it this way).
// Without it, the line is a comment, as for go build: the files with the
// tag are left out. Each package has a tagged and an untagged file: "on"
// means the tag was used. Before, the default loader always read the line
// and the go list loader never did.
func TestYaegiTagsPlugin(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             "module example.com/plug\n\ngo 1.22\n\nrequire example.com/dep v1.0.0\n",
		"vendor/modules.txt": "# example.com/dep v1.0.0\n## explicit; go 1.20\nexample.com/dep\n",
		"vendor/example.com/dep/on.go": `//go:build plugtag

package dep

const Mode = "on"
`,
		"vendor/example.com/dep/off.go": `//go:build !plugtag

package dep

const Mode = "off"
`,
		"mode_on.go": `//go:build plugtag

package plug

const own = "on"
`,
		"mode_off.go": `//go:build !plugtag

package plug

const own = "off"
`,
		"plug.go": `// yaegi:tags plugtag

package plug

import (
	"context"
	"fmt"
	"net/http"

	"example.com/dep"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, own, " ", dep.Mode)
	}), nil
}
`,
	})
	loaders(t, func(t *testing.T, golist bool) {
		if golist {
			if _, err := exec.LookPath("go"); err != nil {
				t.Skip("no go command")
			}
			t.Setenv("GOFLAGS", "-mod=vendor")
		}
		if got := serveOnce(t, Options{Dir: dir, GoList: golist}, "example.com/plug", "/"); got != "off off" {
			t.Errorf("without YaegiTags: body %q, want %q", got, "off off")
		}
		if got := serveOnce(t, Options{Dir: dir, GoList: golist, YaegiTags: true}, "example.com/plug", "/"); got != "on on" {
			t.Errorf("with YaegiTags: body %q, want %q", got, "on on")
		}
	})
}
