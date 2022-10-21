package interp

import (
	"go/ast"
	"go/build"
	"go/parser"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// buildOk returns true if a file or script matches build constraints
// as specified in https://golang.org/pkg/go/build/#hdr-Build_Constraints.
// An error from parser is returned as well.
func (interp *Interpreter) buildOk(ctx *build.Context, name, src string) (bool, error) {
	// Extract comments before the first clause
	f, err := parser.ParseFile(interp.fset, name, src, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return false, err
	}
	for _, g := range f.Comments {
		// in file, evaluate the AND of multiple line build constraints
		for _, line := range strings.Split(strings.TrimSpace(g.Text()), "\n") {
			if !buildLineOk(ctx, line) {
				return false, nil
			}
		}
	}
	setYaegiTags(ctx, f.Comments)
	return true, nil
}

// buildLineOk returns true if line is not a build constraint or
// if build constraint is satisfied.
func buildLineOk(ctx *build.Context, line string) (ok bool) {
	if len(line) < 7 || line[:7] != "+build " {
		return true
	}
	// In line, evaluate the OR of space-separated options
	options := strings.Split(strings.TrimSpace(line[6:]), " ")
	for _, o := range options {
		if ok = buildOptionOk(ctx, o); ok {
			break
		}
	}
	return ok
}

// buildOptionOk return true if all comma separated tags match, false otherwise.
func buildOptionOk(ctx *build.Context, tag string) bool {
	// in option, evaluate the AND of individual tags
	for _, t := range strings.Split(tag, ",") {
		if !buildTagOk(ctx, t) {
			return false
		}
	}
	return true
}

// buildTagOk returns true if a build tag matches, false otherwise
// if first character is !, result is negated.
func buildTagOk(ctx *build.Context, s string) (r bool) {
	not := s[0] == '!'
	if not {
		s = s[1:]
	}
	switch {
	case contains(ctx.BuildTags, s):
		r = true
	case s == ctx.GOOS:
		r = true
	case s == ctx.GOARCH:
		r = true
	case len(s) > 4 && s[:4] == "go1.":
		if n, err := strconv.Atoi(s[4:]); err != nil {
			r = false
		} else {
			r = goMinorVersion(ctx) >= n
		}
	}
	if not {
		r = !r
	}
	return
}

// setYaegiTags scans a comment group for "yaegi:tags tag1 tag2 ..." lines
// and adds the corresponding tags to the interpreter build tags.
func setYaegiTags(ctx *build.Context, comments []*ast.CommentGroup) {
	for _, g := range comments {
		for _, line := range strings.Split(strings.TrimSpace(g.Text()), "\n") {
			if len(line) < 11 || line[:11] != "yaegi:tags " {
				continue
			}

			tags := strings.Split(strings.TrimSpace(line[10:]), " ")
			for _, tag := range tags {
				if !contains(ctx.BuildTags, tag) {
					ctx.BuildTags = append(ctx.BuildTags, tag)
				}
			}
		}
	}
}

func contains(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// goMinorVersion returns the go minor version number.
func goMinorVersion(ctx *build.Context) int {
	current := ctx.ReleaseTags[len(ctx.ReleaseTags)-1]

	v := strings.Split(current, ".")
	if len(v) < 2 {
		panic("unsupported Go version: " + current)
	}

	m, err := strconv.Atoi(v[1])
	if err != nil {
		panic("unsupported Go version: " + current)
	}
	return m
}

// skipFile returns true if file should be skipped.
func skipFile(ctx *build.Context, p string, skipTest bool) bool {
	if !strings.HasSuffix(p, ".go") {
		return true
	}
	p = strings.TrimSuffix(path.Base(p), ".go")
	if pp := filepath.Base(p); strings.HasPrefix(pp, "_") || strings.HasPrefix(pp, ".") {
		return true
	}
	if skipTest && strings.HasSuffix(p, "_test") {
		return true
	}
	i := strings.Index(p, "_")
	if i < 0 {
		return false
	}
	a := strings.Split(p[i+1:], "_")
	last := len(a) - 1
	if last-1 >= 0 {
		switch x, y := a[last-1], a[last]; {
		case x == ctx.GOOS:
			if knownArch[y] {
				return y != ctx.GOARCH
			}
			return false
		case knownOs[x] && knownArch[y]:
			return true
		case knownArch[y] && y != ctx.GOARCH:
			return true
		default:
			return false
		}
	}
	if x := a[last]; knownOs[x] && x != ctx.GOOS || knownArch[x] && x != ctx.GOARCH {
		return true
	}
	return false
}

var knownOs = map[string]bool{
	"aix":       true,
	"android":   true,
	"darwin":    true,
	"dragonfly": true,
	"freebsd":   true,
	"illumos":   true,
	"ios":       true,
	"js":        true,
	"linux":     true,
	"netbsd":    true,
	"openbsd":   true,
	"plan9":     true,
	"solaris":   true,
	"windows":   true,
}

var knownArch = map[string]bool{
	"386":      true,
	"amd64":    true,
	"arm":      true,
	"arm64":    true,
	"loong64":  true,
	"mips":     true,
	"mips64":   true,
	"mips64le": true,
	"mipsle":   true,
	"ppc64":    true,
	"ppc64le":  true,
	"s390x":    true,
	"wasm":     true,
}
