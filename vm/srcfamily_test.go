package vm

import (
	"slices"
	"strings"
	"testing"
)

// TestSrcFamily checks which packages are interpreted with a standard
// library package of Src: its tree when the tree has its own internal
// package (encoding/json of Go 1.27, built on encoding/json/v2 and
// encoding/json/internal), nothing more otherwise.
func TestSrcFamily(t *testing.T) {
	json127 := []string{"encoding/json", "encoding/json/v2", "encoding/json/jsontext",
		"encoding/json/internal", "encoding/json/internal/jsonflags", "fmt", "internal/fmtsort", "reflect"}
	for _, c := range []struct {
		src, paths []string
		want       string
	}{
		{[]string{"encoding/json"}, json127,
			"encoding/json encoding/json/internal encoding/json/internal/jsonflags encoding/json/jsontext encoding/json/v2"},
		{[]string{"encoding/json/v2"}, json127,
			"encoding/json encoding/json/internal encoding/json/internal/jsonflags encoding/json/jsontext encoding/json/v2"},
		// Go 1.26: encoding/json has no internal package.
		{[]string{"encoding/json"}, []string{"encoding/json", "fmt", "internal/fmtsort"}, ""},
		{[]string{"fmt"}, json127, ""},
		// The internal packages that stay native do not count.
		{[]string{"crypto/sha256"}, []string{"crypto/sha256", "crypto/internal/fips140/sha256"}, ""},
		{[]string{"net/http"}, []string{"net/http", "net/http/internal", "net/http/internal/ascii", "net/http/httptrace", "net/url"},
			"net/http net/http/httptrace net/http/internal net/http/internal/ascii"},
		// Not the standard library.
		{[]string{"example.com/m/lib"}, []string{"example.com/m/lib", "example.com/m/lib/internal/x"}, ""},
	} {
		var got []string
		for p := range srcFamily(c.src, c.paths) {
			got = append(got, p)
		}
		slices.Sort(got)
		if s := strings.Join(got, " "); s != c.want {
			t.Errorf("srcFamily(%v): %q, want %q", c.src, s, c.want)
		}
	}
}
