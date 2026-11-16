package vm

import (
	"fmt"
	"strings"
	"testing"
)

// TestMissingEnvCap checks that the names of the missing variables are
// bounded: the plugin chooses them, and they live in the memory of the host,
// outside MaxMemory. 50000 reads of new names of 4 KB keep at most
// maxMissingEnvNames names, each cut to maxMissingEnvNameLen bytes, and
// OnMissingEnv gets each kept name once, then MissingEnvMore once.
func TestMissingEnvCap(t *testing.T) {
	var got []string
	m := &Machine{onMissingEnv: func(name string) { got = append(got, name) }}
	m.SetEnv(nil)
	pad := strings.Repeat("x", 4096)
	for i := 0; i < 50000; i++ {
		m.env.lookup(fmt.Sprintf("N%d_%s", i, pad))
		m.env.lookup("SHORT") // the same name again: reported once
	}
	names := m.MissingEnv()
	if len(names) != maxMissingEnvNames {
		t.Fatalf("MissingEnv keeps %d names, want %d", len(names), maxMissingEnvNames)
	}
	for _, n := range names {
		if len(n) > maxMissingEnvNameLen+len("...") {
			t.Fatalf("a name of %d bytes is kept, want at most %d", len(n), maxMissingEnvNameLen+3)
		}
	}
	if names[0] != "N0_"+pad[:maxMissingEnvNameLen-3]+"..." || names[1] != "SHORT" {
		t.Errorf("first names: %.40q, %.40q", names[0], names[1])
	}
	if len(m.env.reported) != maxMissingEnvNames {
		t.Errorf("%d names in the set, want %d", len(m.env.reported), maxMissingEnvNames)
	}
	if len(got) != maxMissingEnvNames+1 || got[len(got)-1] != MissingEnvMore {
		t.Errorf("OnMissingEnv called %d times (last %.40q), want %d times, the last with MissingEnvMore", len(got), got[len(got)-1], maxMissingEnvNames+1)
	}
}
