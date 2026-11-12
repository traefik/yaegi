package vm

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// go/types reports the parts of an error that have a position of their own
// (the "other declaration of x" of a redeclaration, the steps of an
// invalid recursive type) as errors of their own, whose message starts
// with a tab. gc prints them as one error: the first line, then one line
// per part, which starts with a tab and the position of the part:
//
//	b.go:9:6: invalid recursive type T
//		b.go:9:6: T refers to a.T
//		b.go:9:8: a.T refers to T
//
// The VM reports them as gc does (GOROOT test fixedbugs/issue50788.go):
// a part is joined to the error before it.

// isSubError reports whether msg, a message of go/types, is a part of the
// error before it.
func isSubError(msg string) bool { return strings.HasPrefix(msg, "\t") }

// joinSubError adds the part sub, at position pos, to the message msg.
func joinSubError(msg, pos, sub string) string {
	return msg + "\n\t" + pos + ": " + strings.TrimPrefix(sub, "\t")
}

// joinTypeError joins the type error sub, a part, to head.
func joinTypeError(head *types.Error, sub types.Error) {
	head.Msg = joinSubError(head.Msg, sub.Fset.Position(sub.Pos).String(), sub.Msg)
}

// joinSubErrors joins the parts of the type errors of the packages of
// pkgs, and of their dependencies, to the errors they belong to.
func joinSubErrors(pkgs []*packages.Package) {
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		errs := p.Errors[:0]
		for _, e := range p.Errors {
			if n := len(errs); e.Kind == packages.TypeError && isSubError(e.Msg) &&
				n > 0 && errs[n-1].Kind == packages.TypeError {
				errs[n-1].Msg = joinSubError(errs[n-1].Msg, e.Pos, e.Msg)
				continue
			}
			errs = append(errs, e)
		}
		p.Errors = errs
	})
}
