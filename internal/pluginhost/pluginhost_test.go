package pluginhost

import (
	"testing"

	"github.com/traefik/yaegi/v2/host"
)

// TestOptions checks the options that Traefik-like hosts need for the
// plugins of the catalog and of compare/complex: StubAssembly (zap and grpc
// as restricted plugins), and limits for code the host does not trust.
func TestOptions(t *testing.T) {
	t.Setenv("YAEGIENV_OPTIONS", "secret")
	o := Options()
	// No host variable by default: the host passes the environment as an
	// option (SetEnv with HostEnv or an EnvFilter), as with yaegi. A nil Env
	// would copy the whole host environment.
	if o.Env == nil || len(o.Env) != 0 {
		t.Errorf("Env: %d entries (nil %v), want an empty, non-nil environment", len(o.Env), o.Env == nil)
	}
	if !o.StubAssembly {
		t.Error("StubAssembly is off: zap and grpc do not load as restricted plugins")
	}
	if !o.AOT || !o.StaleAOTFallback || o.AllowStaleAOT {
		t.Errorf("AOT %v, StaleAOTFallback %v, AllowStaleAOT %v: a Go patch release that changes a copied package must load the plugins in default mode, not refuse them nor run stale copies", o.AOT, o.StaleAOTFallback, o.AllowStaleAOT)
	}
	if o.Unrestricted || o.RealProcess || o.UseUnsafe {
		t.Errorf("Unrestricted %v, RealProcess %v, UseUnsafe %v: want all false (the host sets UseUnsafe from the manifest)", o.Unrestricted, o.RealProcess, o.UseUnsafe)
	}
	if !o.PanicOnFatal {
		t.Error("PanicOnFatal is off: a plugin that unlocks an unlocked mutex ends the host")
	}
	if o.Budget == 0 || o.BackgroundRate == 0 || o.MaxMemory == 0 || o.MaxStack == 0 || o.MaxGoroutines == 0 || o.LoadTimeout == 0 || o.Env == nil {
		t.Errorf("a limit is missing: %+v", o)
	}
	// The stack of a goroutine of a Traefik plugin: 256 MiB, below the
	// 1 GiB default of the vm package (a decision of the host profile).
	if o.MaxStack != 256<<20 {
		t.Errorf("MaxStack %d, want %d (256 MiB)", o.MaxStack, 256<<20)
	}
	if err := host.Check(o); err != nil {
		t.Errorf("the options of the catalog host are not safe: %v", err)
	}
}
