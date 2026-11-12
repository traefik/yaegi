package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Build constraints of the copies.
//
// go/packages loads the targets for one build: the GOOS, GOARCH and
// GOEXPERIMENT of the generation. A copy must still have, for every build,
// the same files as its source package: encoding/binary has
// native_endian_little.go on little-endian targets and native_endian_big.go
// on big-endian ones, and encoding/json is v1 (!goexperiment.jsonv2) or a
// wrapper of encoding/json/v2 (goexperiment.jsonv2). So:
//
//   - the build constraint of a copied file keeps its platform part: the
//     tags of platformTag. The other tags (go1.N, custom tags such as
//     purego or cmp_debug) are evaluated for the generation, as before;
//   - the files of the source package that the generation did not select,
//     but that another platform selects (variants), are copied too when
//     they need no type information (cannotCopy): their imports are
//     remapped, nothing else changes;
//   - a variant that cannot be copied (it needs the rewrite, or imports a
//     package that was not loaded) makes the copy refuse the builds that
//     select it: aotgen_guard.go has the constraint of these builds and does
//     not compile there, with a message that says to generate the copy
//     again for that build. A copy never silently differs from its source;
//   - the glue may only name a symbol that the copy declares in every build
//     it supports (checkGlueSymbols).

// knownOS and knownArch are internal/syslist (Go 1.27): the names that
// make a file name suffix a build constraint.
var knownOS = set("aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos")
var unixOS = set("aix android darwin dragonfly freebsd hurd illumos ios linux netbsd openbsd solaris")
var knownArch = set("386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm")

// ports is `go tool dist list` (Go 1.27): the builds that checkGlueSymbols
// and packageExists look at.
var ports = strings.Fields(`aix/ppc64 android/386 android/amd64 android/arm android/arm64
darwin/amd64 darwin/arm64 dragonfly/amd64 freebsd/386 freebsd/amd64 freebsd/arm freebsd/arm64
illumos/amd64 ios/amd64 ios/arm64 js/wasm linux/386 linux/amd64 linux/arm linux/arm64
linux/loong64 linux/mips linux/mips64 linux/mips64le linux/mipsle linux/ppc64 linux/ppc64le
linux/riscv64 linux/s390x netbsd/386 netbsd/amd64 netbsd/arm netbsd/arm64 openbsd/386
openbsd/amd64 openbsd/arm openbsd/arm64 openbsd/ppc64 openbsd/riscv64 plan9/386 plan9/amd64
plan9/arm solaris/amd64 wasip1/wasm windows/386 windows/amd64 windows/arm64`)

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

// platformTag reports whether a build tag depends on the build that
// compiles the copy, not on the generation: GOOS, GOARCH and its feature
// levels (amd64.v3), unix, cgo, GOEXPERIMENT. The compiler (gc, gccgo) is
// not one: the VM needs gc.
func platformTag(tag string) bool {
	if knownOS[tag] || knownArch[tag] {
		return true
	}
	switch tag {
	case "unix", "cgo", "boringcrypto":
		return true
	}
	if strings.HasPrefix(tag, "goexperiment.") {
		return true
	}
	arch, _, ok := strings.Cut(tag, ".")
	return ok && knownArch[arch]
}

// genTag is the value of a tag that is not a platform tag, for the
// generation: the compiler and the release tags (go1.N) of the toolchain,
// false for the rest (aotgen sets no custom tag).
func genTag(tag string) bool {
	if tag == build.Default.Compiler {
		return true
	}
	for _, r := range build.Default.ReleaseTags {
		if r == tag {
			return true
		}
	}
	return false
}

// partial evaluates the tags that are not platform tags. It returns the
// expression left (nil when the result is constant) and the constant.
func partial(x constraint.Expr, val func(string) bool) (constraint.Expr, bool) {
	return reduce(x, func(tag string) (bool, bool) {
		if platformTag(tag) {
			return false, false
		}
		return val(tag), true
	})
}

// reduce evaluates the tags that known knows (ok true) and simplifies. It
// returns the expression left (nil when the result is constant) and the
// constant.
func reduce(x constraint.Expr, known func(tag string) (val, ok bool)) (constraint.Expr, bool) {
	switch x := x.(type) {
	case *constraint.TagExpr:
		v, ok := known(x.Tag)
		if !ok {
			return x, false
		}
		return nil, v
	case *constraint.NotExpr:
		y, v := reduce(x.X, known)
		if y == nil {
			return nil, !v
		}
		return &constraint.NotExpr{X: y}, false
	case *constraint.AndExpr:
		a, av := reduce(x.X, known)
		b, bv := reduce(x.Y, known)
		switch {
		case a == nil && !av, b == nil && !bv:
			return nil, false
		case a == nil:
			return b, bv
		case b == nil:
			return a, av
		}
		return &constraint.AndExpr{X: a, Y: b}, false
	case *constraint.OrExpr:
		a, av := reduce(x.X, known)
		b, bv := reduce(x.Y, known)
		switch {
		case a == nil && av, b == nil && bv:
			return nil, true
		case a == nil:
			return b, bv
		case b == nil:
			return a, av
		}
		return &constraint.OrExpr{X: a, Y: b}, false
	}
	panic(fmt.Sprintf("unknown constraint %T", x))
}

// fileExpr returns the build constraint of the file header: the //go:build
// line, or else the // +build lines. nil when there is none.
func fileExpr(f *ast.File) (constraint.Expr, error) {
	var plus []constraint.Expr
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			switch {
			case constraint.IsGoBuild(c.Text):
				return constraint.Parse(c.Text)
			case constraint.IsPlusBuild(c.Text):
				x, err := constraint.Parse(c.Text)
				if err != nil {
					return nil, err
				}
				plus = append(plus, x)
			}
		}
	}
	return and(plus...), nil
}

// nameExpr returns the constraint of a file name suffix (_linux,
// _s390x, _windows_arm64), as go/build reads it. nil when there is none.
func nameExpr(name string) constraint.Expr {
	name, _, _ = strings.Cut(name, ".")
	i := strings.Index(name, "_")
	if i < 0 {
		return nil
	}
	l := strings.Split(name[i:], "_")
	if n := len(l); n > 0 && l[n-1] == "test" {
		l = l[:n-1]
	}
	n := len(l)
	if n >= 2 && knownOS[l[n-2]] && knownArch[l[n-1]] {
		return and(&constraint.TagExpr{Tag: l[n-2]}, &constraint.TagExpr{Tag: l[n-1]})
	}
	if n >= 1 && (knownOS[l[n-1]] || knownArch[l[n-1]]) {
		return &constraint.TagExpr{Tag: l[n-1]}
	}
	return nil
}

func and(xs ...constraint.Expr) constraint.Expr {
	var out constraint.Expr
	for _, x := range xs {
		switch {
		case x == nil:
		case out == nil:
			out = x
		default:
			out = &constraint.AndExpr{X: out, Y: x}
		}
	}
	return out
}

func or(xs ...constraint.Expr) constraint.Expr {
	var out constraint.Expr
	for _, x := range xs {
		switch {
		case out == nil:
			out = x
		default:
			out = &constraint.OrExpr{X: out, Y: x}
		}
	}
	return out
}

// keptConstraint returns the //go:build expression of a copied file: the
// platform part of its constraint, nil when nothing is left. ok is false
// when the constraint is false whatever the platform (a go1.19 only file,
// a custom tag).
func keptConstraint(f *ast.File, val func(string) bool) (constraint.Expr, bool, error) {
	x, err := fileExpr(f)
	if err != nil || x == nil {
		return nil, true, err
	}
	y, v := partial(x, val)
	if y == nil {
		return nil, v, nil
	}
	return y, true, nil
}

// fixConstraint replaces the build constraint lines of f: the //go:build
// line becomes the kept expression (or a comment when nothing is kept),
// and the // +build lines become comments.
func fixConstraint(f *ast.File, kept constraint.Expr) {
	placed := false
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			switch {
			case constraint.IsGoBuild(c.Text):
				if kept != nil {
					c.Text = "//go:build " + kept.String()
					placed = true
				} else {
					c.Text = "// aotgen: build constraint removed: " + strings.TrimPrefix(c.Text, "//")
				}
			case constraint.IsPlusBuild(c.Text):
				if kept != nil && !placed {
					c.Text = "//go:build " + kept.String()
					placed = true
				} else {
					c.Text = "// aotgen: legacy build line removed"
				}
			}
		}
	}
}

// buildOf is one build: a port and the values of the other platform tags.
type buildOf struct {
	goos, goarch string
	tags         map[string]bool
}

func (b buildOf) has(tag string) bool {
	switch {
	case tag == b.goos || tag == b.goarch:
		return true
	case tag == "linux" && b.goos == "android", tag == "solaris" && b.goos == "illumos", tag == "darwin" && b.goos == "ios":
		return true
	case tag == "unix":
		return unixOS[b.goos]
	}
	return b.tags[tag]
}

// counterexample returns a build where x is false: a port and values for
// the other tags that x names (cgo, goexperiment.X, amd64.v3...). ok is
// false when x holds in every build (or x is nil).
//
// A tag gets a value only while the expression left still depends on it.
// Before, aotgen tried every combination of the tags, 2^n builds per port,
// and stopped above 12 tags: internal/goexperiment of Go 1.27 has 20 (one
// file per experiment and value). Here its "exp_X_on.go || exp_X_off.go ||
// ..." is true as soon as the first tag has a value.
func counterexample(x constraint.Expr) (buildOf, bool) {
	if x == nil {
		return buildOf{}, false
	}
	for _, p := range ports {
		goos, goarch, _ := strings.Cut(p, "/")
		b := buildOf{goos, goarch, map[string]bool{}}
		if b.falsify(x) {
			return b, true
		}
	}
	return buildOf{}, false
}

// falsify looks for values of the tags of x that are not in b.tags (and
// are not GOOS, GOARCH or unix) that make x false. It sets them in b.tags
// and returns true when it finds some.
func (b buildOf) falsify(x constraint.Expr) bool {
	y, v := reduce(x, func(tag string) (bool, bool) {
		if knownOS[tag] || knownArch[tag] || tag == "unix" {
			return b.has(tag), true
		}
		v, ok := b.tags[tag]
		return v, ok
	})
	if y == nil {
		return !v
	}
	tag := firstTag(y)
	for _, v := range []bool{false, true} {
		b.tags[tag] = v
		if b.falsify(y) {
			return true
		}
	}
	delete(b.tags, tag)
	return false
}

// firstTag returns the first tag of x, from the left.
func firstTag(x constraint.Expr) string {
	switch x := x.(type) {
	case *constraint.TagExpr:
		return x.Tag
	case *constraint.NotExpr:
		return firstTag(x.X)
	case *constraint.AndExpr:
		return firstTag(x.X)
	case *constraint.OrExpr:
		return firstTag(x.X)
	}
	panic(fmt.Sprintf("unknown constraint %T", x))
}

// variant is a file of the source package for other builds.
type variant struct {
	name string
	f    *ast.File
	when constraint.Expr // file name and kept constraint
	kept constraint.Expr // kept //go:build expression
	why  string          // why it cannot be copied ("" when it is copied)
}

// variants parses the files of the package that the generation did not
// select but that other builds select.
func (g *gen) variants(pi *pinfo) ([]*variant, error) {
	var out []*variant
	for _, path := range pi.p.IgnoredFiles {
		name := filepath.Base(path)
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(g.fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		kept, ok, err := keptConstraint(f, genTag)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if !ok {
			continue // ignore, go1.19 only, a custom tag...
		}
		v := &variant{name: name, f: f, when: and(nameExpr(name), kept), kept: kept}
		if v.when == nil {
			// Selected by every build, but not by this one: the
			// generation set a tag that aotgen cannot see.
			return nil, fmt.Errorf("%s: file not selected, but its constraint holds", name)
		}
		v.why = g.cannotCopy(pi, f)
		out = append(out, v)
	}
	return out, nil
}

// cannotCopy says why a variant cannot be copied without type information,
// "" when it can: the rewrite of a converted package works on types
// (assertions, type switches, interface comparisons, reflect), and the
// imports are remapped from the loaded packages only.
func (g *gen) cannotCopy(pi *pinfo, f *ast.File) string {
	for _, is := range f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		if p == "unsafe" {
			continue
		}
		if p == "C" {
			return "cgo"
		}
		if isReflectPath(p) && pi.kind == kConvert {
			return "imports " + p
		}
		if g.pkgs[p] == nil {
			return "imports " + p + ", which this build does not load"
		}
	}
	if len(bodyless(f)) > 0 {
		return "bodyless functions"
	}
	if pi.kind != kConvert {
		return ""
	}
	why := ""
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.TypeAssertExpr:
			why = "type assertion"
		case *ast.TypeSwitchStmt:
			why = "type switch"
		case *ast.SwitchStmt:
			if n.Tag != nil {
				why = "switch with a tag"
			}
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				why = "comparison"
			}
		}
		return why == ""
	})
	if why != "" {
		return why + " (the rewrite needs type information)"
	}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:linkname") {
				return "go:linkname"
			}
		}
	}
	return ""
}

// writeVariants copies the variants that can be copied, and writes
// aotgen_guard.go for the builds of the others. It returns the copied
// variants and the guard constraint (nil when there is no guard).
func (g *gen) writeVariants(pi *pinfo, vs []*variant) ([]*variant, constraint.Expr, error) {
	var copied []*variant
	var guard []constraint.Expr
	var names []string
	seen := map[string]bool{}
	for _, v := range vs {
		if v.why != "" {
			if !seen[v.when.String()] {
				seen[v.when.String()] = true
				guard = append(guard, v.when)
			}
			names = append(names, "//\t"+v.name)
			pi.notes = append(pi.notes, fmt.Sprintf("variant %s (%s) not copied: %s; aotgen_guard.go refuses these builds", v.name, v.when, v.why))
			continue
		}
		rw := &rewriter{g: g, pi: pi, f: v.f, kept: v.kept}
		rw.fixFile()
		src, err := rw.print(v.name)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", v.name, err)
		}
		if err := os.WriteFile(filepath.Join(pi.dir, v.name), src, 0o644); err != nil {
			return nil, nil, err
		}
		pi.notes = append(pi.notes, fmt.Sprintf("variant %s (%s) copied", v.name, v.when))
		copied = append(copied, v)
	}
	if len(guard) == 0 {
		return copied, nil, nil
	}
	x := or(guard...)
	src := fmt.Sprintf(`// Code generated by aotgen. DO NOT EDIT.

//go:build %s

package %s

// This copy of %s was generated for another build. It does not have
// these files, which this build selects in the source package:
%s
// Generate the copy again with the GOOS, GOARCH and GOEXPERIMENT of this
// build (docs/AOTGEN.md).
var _ = aotgen_copy_generated_for_another_build_see_aotgen_guard_go
`, x, pi.p.Name, pi.p.PkgPath, strings.Join(names, "\n"))
	return copied, x, os.WriteFile(filepath.Join(pi.dir, "aotgen_guard.go"), []byte(src), 0o644)
}

// declared returns the package-level names that f declares: "Name" and,
// for methods, "Type.Method".
func declared(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) == 0 {
				out = append(out, d.Name.Name)
				continue
			}
			t := d.Recv.List[0].Type
			if s, ok := t.(*ast.StarExpr); ok {
				t = s.X
			}
			switch x := t.(type) {
			case *ast.IndexExpr:
				t = x.X
			case *ast.IndexListExpr:
				t = x.X
			}
			if id, ok := t.(*ast.Ident); ok {
				out = append(out, id.Name+"."+d.Name.Name)
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					out = append(out, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	return out
}

// checkGlueSymbols makes aotgen stop when the glue names a symbol that the
// copy does not declare in every build it supports (exists, outside
// guard): the copy would not compile there. files maps each file of the copy to its
// constraint (file name and kept //go:build).
func checkGlueSymbols(glue []byte, files map[string]constraint.Expr, decls map[string][]string, guard, exists constraint.Expr) error {
	where := map[string][]constraint.Expr{}
	always := map[string]bool{}
	for name, x := range files {
		for _, d := range decls[name] {
			if x == nil {
				always[d] = true
				continue
			}
			where[d] = append(where[d], x)
		}
	}
	// A build is fine when the guard refuses it, when it does not have the
	// package, or when one of the files that declare the symbol is there.
	var fine []constraint.Expr
	if guard != nil {
		fine = append(fine, guard)
	}
	if exists != nil {
		fine = append(fine, &constraint.NotExpr{X: exists})
	}
	var bad []string
	for d, xs := range where {
		if always[d] {
			continue
		}
		short := d
		if _, m, ok := strings.Cut(d, "."); ok {
			short = m
		}
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(short) + `\b`).Match(glue) {
			continue
		}
		if b, ok := counterexample(or(append(slices.Clone(fine), xs...)...)); ok {
			bad = append(bad, fmt.Sprintf("%s (missing on %s/%s %v)", d, b.goos, b.goarch, b.tags))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("the glue names symbols that some builds do not declare: %s", strings.Join(bad, "; "))
	}
	return nil
}

// packageExists returns the builds that have the package: the builds that
// select one of its files (nil when it is all of them). encoding/json/v2
// exists only with GOEXPERIMENT=jsonv2: its glue and its import in aot/all
// must have the same constraint, as a native import of it does not build
// elsewhere.
func packageExists(files map[string]constraint.Expr, vs []*variant) (constraint.Expr, error) {
	var xs []constraint.Expr
	seen := map[string]bool{}
	add := func(x constraint.Expr) {
		if x != nil && !seen[x.String()] {
			seen[x.String()] = true
			xs = append(xs, x)
		}
	}
	names := make([]string, 0, len(files))
	for name, x := range files {
		if x == nil {
			return nil, nil
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		add(files[name])
	}
	for _, v := range vs {
		add(v.when) // copied or refused by the guard: the package exists there
	}
	x := or(xs...)
	if _, ok := counterexample(x); ok {
		return x, nil
	}
	return nil, nil
}

// writeAllConstrained imports in aot/all the copies that only some builds
// have (packageExists), one file per build constraint.
func (g *gen) writeAllConstrained() error {
	old, _ := filepath.Glob(filepath.Join(g.outDir, "all", "constrained*.go"))
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	byExpr := map[string][]string{}
	for _, pi := range g.sorted() {
		if _, internal := internalParent(pi.p.PkgPath); internal || pi.exists == nil {
			continue
		}
		if pi.kind == kConvert && (pi.built || pi.buildErr == "") {
			byExpr[pi.exists.String()] = append(byExpr[pi.exists.String()], pi.aotPath)
		}
	}
	exprs := make([]string, 0, len(byExpr))
	for x := range byExpr {
		exprs = append(exprs, x)
	}
	sort.Strings(exprs)
	for i, x := range exprs {
		var b strings.Builder
		fmt.Fprintf(&b, "// Code generated by aotgen. DO NOT EDIT.\n\n//go:build %s\n\npackage all\n\nimport (\n", x)
		for _, p := range byExpr[x] {
			fmt.Fprintf(&b, "\t_ %q\n", p)
		}
		b.WriteString(")\n")
		name := fmt.Sprintf("constrained%d.go", i+1)
		if err := os.WriteFile(filepath.Join(g.outDir, "all", name), []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}
