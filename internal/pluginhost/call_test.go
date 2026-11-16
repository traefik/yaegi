package pluginhost

import (
	"context"
	"testing"

	"github.com/traefik/yaegi/v2/vm"
)

// call calls the function name of p and returns its results as native values.
func call(t *testing.T, p *vm.Plugin, name string, args ...any) []any {
	t.Helper()
	res, err := p.Call(context.Background(), name, args...)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := make([]any, len(res))
	for i, r := range res {
		if out[i], err = r.Interface(); err != nil {
			t.Fatalf("%s: result %d: %v", name, i, err)
		}
	}
	return out
}
