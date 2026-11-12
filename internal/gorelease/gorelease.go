// Package gorelease holds what the generators of yaegi v2 (cmd/stdexport,
// cmd/stdsyms, cmd/aotgen) share about Go releases: the window of releases
// that yaegi v2 supports, the release of a Go version, the build constraint
// that limits generated code to one release, and the guard file that stops
// the build on a release outside the window.
//
// yaegi v2 supports two releases, N-1 and N (Window): the generated data of
// each one is kept side by side, each file limited to its release by its
// build constraint, so a host can move to N when it wants.
//
// It must not import the generated packages: the generators run with a new
// Go release exactly when the guard of those packages stops their build.
package gorelease

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Platforms are the GOOS/GOARCH pairs with embedded standard library types
// (package stdtypes) and a syscall table (package stdsyms). See
// stdtypes.Platforms. They are the targets of the Traefik releases (linux/arm
// covers arm v6 and v7: GOARM does not change the API of the standard
// library). A binary only embeds the data of its own platform.
var Platforms = []string{
	"linux/amd64", "linux/arm64", "linux/386", "linux/arm",
	"linux/ppc64le", "linux/s390x", "linux/riscv64",
	"darwin/amd64", "darwin/arm64",
	"windows/amd64", "windows/arm64", "windows/386",
	"freebsd/amd64", "freebsd/386",
	"openbsd/amd64", "openbsd/386", "openbsd/riscv64",
}

// Window are the minor numbers of the Go releases that yaegi v2 supports,
// oldest first: N-1 and N. Each generated data set (stdtypes, stdsyms,
// aot, vm/zz_natsigs_*.go) exists once per release of the window. To follow
// a new release, add it here, drop the oldest, generate the data with the
// new toolchain and remove the files of the dropped release
// (docs/DEVELOPMENT.md, "Following a new Go release").
var Window = []int{26, 27}

// Filled are the releases that YAEGI_FORWARD puts in Window only so that it
// has no gap: they have no generated data (forwardWindow).
var Filled []int

// forward is YAEGI_FORWARD=1 (the CI job "next Go", on the latest release, a
// release candidate or Go tip): a toolchain newer than the window joins it,
// so the job generates the data of its release next to the others and tests
// it before yaegi v2 follows it. Go tip ("devel go1.29-abcdef ...") counts as
// its release.
var forward = os.Getenv("YAEGI_FORWARD") == "1"

func init() {
	if forward {
		Window, Filled = forwardWindow(Window, runtime.Version())
	}
}

// forwardWindow returns window with the release of the toolchain version v
// when it is newer, and the releases between them (Filled): the window has
// no gap (WindowConstraint and Guard need it). Go tip is often two releases
// ahead of the window (it moves to N+2 when the N+1 release candidates
// start).
func forwardWindow(window []int, v string) (w, filled []int) {
	minor, ok := Minor(develRelease(v))
	hi := window[len(window)-1]
	if !ok || minor <= hi {
		return window, nil
	}
	w = slices.Clone(window)
	for m := hi + 1; m <= minor; m++ {
		w = append(w, m)
		if m < minor {
			filled = append(filled, m)
		}
	}
	return w, filled
}

// develRelease returns "go1.29-abcdef ..." for the version of Go tip
// "devel go1.29-abcdef ...", and v for another version.
func develRelease(v string) string {
	return strings.TrimPrefix(v, "devel ")
}

// HasData reports whether release minor of Window has generated data: not a
// release of Filled.
func HasData(minor int) bool {
	return !slices.Contains(Filled, minor)
}

// OutsideWindow reports whether the generated file or directory name is for
// a release outside Window: its base name has the prefix of a release
// ("go1_25_data_linux_amd64.go", "zz_natsigs_go1_25.go", "go1_25"). A name
// without such a prefix is not for a release: false. The generators remove
// these files, and TestSupportWindow checks that none is left.
func OutsideWindow(name string) bool {
	base := name
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	i := strings.Index(base, "go1_")
	if i < 0 {
		return false
	}
	var minor int
	if _, err := fmt.Sscanf(base[i:], "go1_%d", &minor); err != nil {
		return false
	}
	return !InWindow("go1." + strconv.Itoa(minor))
}

// InWindow reports whether version v (or release) is a release of Window.
func InWindow(v string) bool {
	minor, ok := Minor(v)
	if !ok {
		return false
	}
	for _, w := range Window {
		if w == minor {
			return true
		}
	}
	return false
}

// WindowNames returns the releases of Window: "go1.26 and go1.27".
func WindowNames() string {
	var out []string
	for _, w := range Window {
		out = append(out, "go1."+strconv.Itoa(w))
	}
	return strings.Join(out, " and ")
}

// WindowConstraint returns the build constraint of the releases of Window:
// "go1.26 && !go1.28". Window has consecutive releases.
func WindowConstraint() string {
	return fmt.Sprintf("go1.%d && !go1.%d", Window[0], Window[len(Window)-1]+1)
}

// Prefix returns the prefix of the names of the generated files of the
// release of version v: "go1_26" for "go1.26.8".
func Prefix(v string) string {
	return strings.ReplaceAll(Release(v), ".", "_")
}

// StripExperiments returns v without the GOEXPERIMENT suffix that
// runtime.Version() has in a toolchain built with experiments:
// "go1.26.9" for "go1.26.9 X:boringcrypto,jsonv2". Other strings come back
// as is.
func StripExperiments(v string) string {
	if i := strings.Index(v, " X:"); i >= 0 {
		return v[:i]
	}
	return v
}

// Release returns the release of a Go version: "go1.26" for "go1.26.8",
// "go1.27rc1" or "go1.27". Other strings (devel builds) come back as is.
func Release(v string) string {
	minor, ok := Minor(v)
	if !ok {
		return v
	}
	return "go1." + strconv.Itoa(minor)
}

// Minor returns 26 for "go1.26", "go1.26.8" or "go1.26rc1".
func Minor(v string) (int, bool) {
	if forward {
		v = develRelease(v)
	}
	rest, ok := strings.CutPrefix(v, "go1.")
	if !ok {
		return 0, false
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	if n == 0 {
		return 0, false
	}
	minor, err := strconv.Atoi(rest[:n])
	return minor, err == nil
}

// Patch returns 8 for "go1.26.8" (also "go1.26.8 X:jsonv2", see
// StripExperiments), and 0 for "go1.26" (a release candidate or the first
// release). ok is false when v is not a release version.
func Patch(v string) (patch int, ok bool) {
	v = StripExperiments(v)
	if _, ok := Minor(v); !ok {
		return 0, false
	}
	rest := strings.TrimPrefix(v, "go1.")
	i := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if i < 0 {
		return 0, true // "go1.26"
	}
	if !strings.HasPrefix(rest[i:], ".") {
		return 0, true // "go1.27rc1": before the first release
	}
	p, err := strconv.Atoi(rest[i+1:])
	if err != nil {
		return 0, false
	}
	return p, true
}

// Compare compares the Go versions a and b: -1 when a is older than b, 0
// when they are the same version, +1 when a is newer. It orders by release,
// then betas before release candidates before the patch releases:
// go1.27beta1 < go1.27rc1 < go1.27rc2 < go1.27.0 < go1.27.1. "go1.27" (the
// name of the first release before Go 1.21) is go1.27.0. GOEXPERIMENT
// suffixes are ignored. Versions that do not parse are compared as
// strings after the release.
func Compare(a, b string) int {
	a, b = StripExperiments(a), StripExperiments(b)
	ma, aok := Minor(a)
	mb, bok := Minor(b)
	if !aok || !bok {
		return strings.Compare(a, b)
	}
	if ma != mb {
		return cmpInt(ma, mb)
	}
	ka, na, aok := stage(a)
	kb, nb, bok := stage(b)
	if !aok || !bok {
		return strings.Compare(a, b)
	}
	if ka != kb {
		return cmpInt(ka, kb)
	}
	return cmpInt(na, nb)
}

// stage returns the stage of version v of a release (0 for a beta, 1 for a
// release candidate, 2 for a release) and its number (the beta or release
// candidate number, or the patch number).
func stage(v string) (kind, n int, ok bool) {
	rest := strings.TrimPrefix(v, "go1.")
	i := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if i < 0 {
		return 2, 0, true // "go1.26"
	}
	for _, s := range []struct {
		prefix string
		kind   int
	}{{"beta", 0}, {"rc", 1}, {".", 2}} {
		if num, found := strings.CutPrefix(rest[i:], s.prefix); found {
			n, err := strconv.Atoi(num)
			if err != nil || n < 0 {
				return 0, 0, false
			}
			return s.kind, n, true
		}
	}
	return 0, 0, false
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Constraint returns the build constraint of the code generated with the
// release of version v: "go1.26 && !go1.27" for "go1.26.8". The release
// must be in Window.
func Constraint(v string) (string, error) {
	minor, ok := Minor(v)
	if !ok {
		return "", fmt.Errorf("%q is not a Go release: generate with a released Go toolchain", v)
	}
	if !InWindow(v) {
		return "", fmt.Errorf("%s is not in the window of yaegi v2 (%s): add it to gorelease.Window first", Release(v), WindowNames())
	}
	return fmt.Sprintf("go1.%d && !go1.%d", minor, minor+1), nil
}

// Guard returns the source of the guard file of package pkg, whose code is
// generated for the releases of Window: a file that is only built by a
// release outside the window, and then does not compile, with a message
// that says what to do. regen is the command that regenerates the package
// in a yaegi v2 checkout. A Go update then stops the build of the host
// instead of failing when a plugin is loaded.
func Guard(generator, pkg, regen string) []byte {
	lo, hi := Window[0], Window[len(Window)-1]
	msg := fmt.Sprintf("yaegi v2: package %s was generated for %s and does not build with another Go release: use a yaegi v2 version made for your Go release, or regenerate it in a yaegi v2 checkout: %s", pkg, WindowNames(), regen)
	return []byte(fmt.Sprintf(`// Code generated by %s. DO NOT EDIT.

//go:build !go1.%d || go1.%d

package %s

// This file is only built by a Go release outside %s, the releases this
// package was generated for. It does not compile on purpose: the generated
// code follows the releases of the standard library (their API, their
// internals, their security fixes), so a Go update must stop the build,
// not fail when a plugin is loaded. The error message says what to do.
var _ int = %q
`, generator, lo, hi+1, pathBase(pkg), WindowNames(), msg))
}

func pathBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
