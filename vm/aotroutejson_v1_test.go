//go:build !goexperiment.jsonv2

package vm

import (
	"encoding/json"
	"strings"
	"testing"
	"unsafe"
	"weak"
)

// TestJSONTwinCleanup checks that the cleanup of a dead Decoder does not
// remove the twin of a new Decoder at the same address: the cleanup can run
// after the new Decoder recorded its twin (the address is reused once the
// old one is freed).
func TestJSONTwinCleanup(t *testing.T) {
	d := json.NewDecoder(strings.NewReader(""))
	k := uintptr(unsafe.Pointer(d))
	old := &jsonTwinEntry{dec: weak.Make(d)}
	cur := &jsonTwinEntry{dec: weak.Make(d)}
	jsonTwins.Store(k, cur) // the twin of the new Decoder at the same address
	defer jsonTwins.Delete(k)
	dropJSONTwin(k, old) // the late cleanup of the dead Decoder
	if e, ok := jsonTwins.Load(k); !ok || e != cur {
		t.Fatalf("the cleanup of the old Decoder removed the twin of the new one")
	}
	dropJSONTwin(k, cur)
	if _, ok := jsonTwins.Load(k); ok {
		t.Fatal("the cleanup of the Decoder did not remove its own twin")
	}
}
