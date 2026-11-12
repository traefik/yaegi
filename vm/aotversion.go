package vm

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/traefik/yaegi/v2/internal/gorelease"
	"github.com/traefik/yaegi/v2/internal/gosources"
)

// The AOT copies of the standard library (aot/fmt, aot/encoding/json,
// aot/html/template...) are copies of the sources of one Go version. They
// follow the internals of that release, and they only have the security
// fixes of that patch release: a host built with a newer patch release gets
// the fixes in its native standard library, not in the copies, and
// govulncheck does not see the copies.
//
// So the copies record the Go version they were made with
// (Package.GoVersion), and Load and RunMain check it in AOT mode:
//   - another release (go1.27 host, go1.26 copies): always an error. The
//     guard file of aot/all already stops the build in this case.
//   - the same release, and the host is a newer patch release (go1.26.9
//     host, go1.26.8 copies): fine when the manifest of the Go sources
//     (aot/all/gosources.txt, RegisterAOTGoSources) says that the newer
//     patch release did not change the sources of any copied package: the
//     copies are then the same code as the standard library of the host.
//     Otherwise (sources changed, or a version not in the manifest) an
//     error, unless the host accepts it with Options.AllowStaleAOT. Fail
//     closed: the fix is to regenerate the copies (sh scripts/aotgen.sh)
//     with the Go version of the host, or to add the new patch release to
//     the manifest (go run ./cmd/aotgosrc -update) when it changed nothing.
//   - the same release, and the host is the same or an older patch release:
//     fine (the copies have at least the fixes of the host).
//
// The policy (docs/DEVELOPMENT.md, "Go patch releases"): each Go patch release of the
// host needs a yaegi v2 whose copies were made with it or a newer one, or
// whose manifest says that it changed no copied package. A host checks this once with CheckAOT, at start-up or in its
// own tests, so that a Go update without a yaegi v2 update fails there and not
// at the first plugin load.
//
// A host that prefers to load the plugin anyway, without the copies, sets
// Options.StaleAOTFallback: when the check fails, the load runs in default
// mode instead of AOT mode (aotGate). No copy runs then, so no fix is
// missed: the standard library is the native one of the host. The cost is
// the behavior of AOT mode (the copies see the methods of the plugin
// types; native packages do not), and the load says it in Plugin.Notes.

// CheckAOT returns the error that Load and RunMain return in AOT mode when
// the registered AOT copies (import github.com/traefik/yaegi/v2/aot/all)
// cannot run in this host, or nil. allowStale is Options.AllowStaleAOT.
//
// A host that loads plugins in AOT mode calls it in a test of its own
// (CheckAOT(false) must be nil), so that its CI fails when its Go toolchain
// moves to a newer patch release than the copies, and not the first plugin
// load in production. It is the same check as Load: there is no need to
// call it before Load.
func CheckAOT(allowStale bool) error {
	return checkAOTVersion(runtime.Version(), allowStale)
}

// aotGate applies the version check of the AOT copies to a load that asks
// for AOT mode (aot, or ModeAOT in modes). It returns the AOT flag and the
// modes of the load. When the copies cannot run in a host built with Go
// version hostGo, it returns the error of checkAOTVersion, or, with
// fallback (Options.StaleAOTFallback), AOT mode off (aot false, no ModeAOT
// in the modes) and a note for Plugin.Notes. modes itself is not changed.
//
// The fallback is the whole default mode, not one package at a time: the
// copies call each other (html/template.FuncMap is the FuncMap of the copy
// of text/template, almost every copy calls the copies of fmt and errors),
// so a native package next to a copy would mix two sets of types. In
// default mode, the AOT triggers (stdAOTModes) do not use the copies
// either: they check the same version.
func aotGate(hostGo string, aot bool, modes map[string]Mode, allowStale, fallback bool) (bool, map[string]Mode, string, error) {
	if !aot && !hasMode(modes, ModeAOT) {
		return aot, modes, "", nil
	}
	err := checkAOTVersion(hostGo, allowStale)
	if err == nil {
		return aot, modes, "", nil
	}
	if !fallback {
		return aot, modes, "", err
	}
	out := make(map[string]Mode, len(modes))
	for k, v := range modes {
		if v != ModeAOT {
			out[k] = v
		}
	}
	note := fmt.Sprintf("AOT mode is off for this load (StaleAOTFallback): %v. The plugin runs in default mode: native standard library (with the fixes of the host), other libraries interpreted", err)
	return false, out, note, nil
}

// AOTGoVersions returns the Go versions that the registered AOT copies were
// made with, sorted (one version when the copies come from one yaegi v2
// release). A host may log them with runtime.Version().
func AOTGoVersions() []string {
	regMu.RLock()
	seen := map[string]bool{}
	for _, p := range aotPkgs {
		if p.GoVersion != "" {
			seen[p.GoVersion] = true
		}
	}
	regMu.RUnlock()
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

var (
	goSources    gosources.Manifest // RegisterAOTGoSources, under regMu
	goSourcesErr error
)

// RegisterAOTGoSources registers the manifest of the GOROOT sources of the
// copied standard packages (internal/gosources, aot/all/gosources.txt).
// aot/all calls it in its init. A manifest that does not parse is ignored:
// the check then fails closed, and its error says why.
func RegisterAOTGoSources(data []byte) {
	m, err := gosources.Parse(data)
	regMu.Lock()
	defer regMu.Unlock()
	if err != nil {
		goSourcesErr = err
		return
	}
	if goSources == nil {
		goSources = gosources.Manifest{}
	}
	for v, pkgs := range m {
		goSources[v] = pkgs
	}
}

// checkAOTVersion returns an error when the registered AOT copies cannot run
// in a host built with Go version host (see above). allowStale accepts copies
// from an older patch release of the same Go release.
func checkAOTVersion(host string, allowStale bool) error {
	regMu.RLock()
	byVersion := map[string][]string{}
	for path, p := range aotPkgs {
		if p.GoVersion != "" {
			byVersion[p.GoVersion] = append(byVersion[p.GoVersion], path)
		}
	}
	regMu.RUnlock()
	versions := make([]string, 0, len(byVersion))
	for v := range byVersion {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	for _, v := range versions {
		if err := aotVersionError(host, v, allowStale); err != nil {
			paths := byVersion[v]
			sort.Strings(paths)
			if len(paths) > 3 {
				paths = append(paths[:3:3], "...")
			}
			return fmt.Errorf("%v (AOT copies of %s)", err, strings.Join(paths, ", "))
		}
	}
	return nil
}

// aotVersionError applies the policy above to a host built with Go version
// host and AOT copies made with Go version copies.
func aotVersionError(host, copies string, allowStale bool) error {
	// A toolchain built with GOEXPERIMENT gives "go1.26.9 X:jsonv2": the
	// experiments do not change the patch release.
	host, copies = gorelease.StripExperiments(host), gorelease.StripExperiments(copies)
	if gorelease.Release(host) != gorelease.Release(copies) {
		return fmt.Errorf("vm: the AOT copies were made with %s and the host is built with %s: AOT mode needs copies made for %s (sh scripts/aotgen.sh)",
			copies, host, gorelease.Release(host))
	}
	if allowStale || host == copies {
		return nil
	}
	_, hok := gorelease.Patch(host)
	_, cok := gorelease.Patch(copies)
	if !hok || !cok {
		return fmt.Errorf("vm: the AOT copies were made with %s and the host is built with %s: regenerate the copies (sh scripts/aotgen.sh) or set AllowStaleAOT", copies, host)
	}
	// A release candidate comes before the first patch release of its
	// release: go1.27rc2 < go1.27.0 (the order of gosources.Less too).
	if gorelease.Compare(host, copies) > 0 {
		regMu.RLock()
		same, changed, known := goSources.Same(copies, host)
		perr := goSourcesErr
		regMu.RUnlock()
		if same {
			return nil // the newer patch release changed no copied package
		}
		why := fmt.Sprintf("%s is not in the manifest of the Go sources (go run ./cmd/aotgosrc -update)", host)
		switch {
		case perr != nil:
			why = fmt.Sprintf("the manifest of the Go sources does not parse: %v", perr)
		case known:
			why = fmt.Sprintf("%s changed the sources of %s", host, strings.Join(changed, ", "))
		}
		return fmt.Errorf("vm: the AOT copies of the standard library were made with %s, older than the Go version of the host (%s), and %s: they miss the fixes of the newer patch releases. Regenerate the copies with %s (sh scripts/aotgen.sh), or set AllowStaleAOT to load anyway", copies, host, why, host)
	}
	return nil
}
