package vm

// Options.IgnoreSoftErrors: the soft errors of go/types about unused names
// do not refuse a plugin: "declared and not used", "imported and not used",
// "label declared and not used", and "no new variables on left side of :="
// (an assignment then). gc refuses such code, but yaegi does not report
// unused names, so a plugin author who only tried the plugin with yaegi
// never saw them: one plugin of the catalog has an unused variable. The
// type information stays complete, so the SSA build and the run are the
// same as if the names were used.
//
// The other soft errors (types.Error.Soft) still refuse the plugin: a type
// argument that does not satisfy its constraint, a constraint interface
// used as a type, a range over a value that cannot be ranged over. The
// type information is complete there too, but the program is not valid Go:
// Inc[string] of "func Inc[T ~int](x T) T { return x + 1 }" would run
// "a" + 1.
//
// Options.IgnoreUnusedExprs: the error "x is not used" of go/types (an
// expression statement whose value is not used, like "c.S" or "len(s)")
// does not refuse a plugin. It is not a soft error for go/types, but the
// expression is fully checked before the error is reported, so the type
// information is complete too: the expression runs and its value is
// dropped, as in yaegi. gc refuses it. The other errors of an expression
// statement still refuse the plugin: a built-in that is not called
// ("len" alone) and a type used as an expression.

import (
	"errors"
	"go/types"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// typeErrPolicy says which type errors of go/types do not refuse a plugin.
type typeErrPolicy struct {
	unusedNames bool // Options.IgnoreSoftErrors
	unusedExprs bool // Options.IgnoreUnusedExprs
}

// typeErrPolicy returns the type errors that opts ignores.
func (opts Options) typeErrPolicy() typeErrPolicy {
	return typeErrPolicy{unusedNames: opts.IgnoreSoftErrors, unusedExprs: opts.IgnoreUnusedExprs}
}

// any reports whether p ignores some type errors.
func (p typeErrPolicy) any() bool { return p.unusedNames || p.unusedExprs }

// ignores reports whether p ignores the type error te.
func (p typeErrPolicy) ignores(te types.Error) bool {
	return (p.unusedNames && isUnusedErr(te)) || (p.unusedExprs && isUnusedExprErr(te))
}

// ignoresErr reports whether err is a type error of go/types that p
// ignores.
func (p typeErrPolicy) ignoresErr(err error) bool {
	var te types.Error
	return errors.As(err, &te) && p.ignores(te)
}

// option returns the name of the option of Options that accepts err, among
// the ones p ignores: "" when none does.
func (p typeErrPolicy) option(err error) string {
	var te types.Error
	switch {
	case !errors.As(err, &te):
		return ""
	case p.unusedNames && isUnusedErr(te):
		return "IgnoreSoftErrors"
	case p.unusedExprs && isUnusedExprErr(te):
		return "IgnoreUnusedExprs"
	}
	return ""
}

// unusedMsg matches the messages of go/types for the unused names (see
// go/types: stmt.go, resolver.go, labels.go, assignments.go). A message
// that a later Go release words differently refuses the plugin again.
var unusedMsg = regexp.MustCompile(`^(declared and not used: \S+|\S+ declared and not used|label \S+ declared and not used|"[^"]+" imported (as \S+ )?and not used|no new variables? on left side of :=)$`)

// isUnusedErr reports whether te is a soft error about an unused name.
func isUnusedErr(te types.Error) bool {
	return te.Soft && unusedMsg.MatchString(te.Msg)
}

// unusedExprMsg matches the message of go/types for an expression
// statement whose value is not used (go/types: stmt.go, code UnusedExpr):
// the expression, then in parentheses its mode (operand.go: operandString)
// and its type, then "is not used". Only the modes of a checked value are
// accepted: "nil is not used", "len (built-in) must be called" and "int
// (type) is not an expression" refuse the plugin.
var unusedExprMsg = regexp.MustCompile(`^.+ \((untyped (bool|int|rune|float|complex|string) )?(constant|variable|map index expression|value|comma, ok expression|comma, error expression)( .+)?\) is not used$`)

// invalidOperandMsg ends the message of go/types for an expression
// statement whose expression is invalid: "x (invalid operand) is not
// used". go/types reports it when no error was reported before (errors.go,
// a follow-on error otherwise), so the error that made the expression
// invalid may be elsewhere (in a dependency). It has no type information:
// never ignored. The suffix is checked apart from unusedExprMsg because the
// expression may hold a string literal that looks like a mode and a type.
const invalidOperandMsg = " (invalid operand) is not used"

// isUnusedExprErr reports whether te is the error of an expression
// statement whose value is not used.
func isUnusedExprErr(te types.Error) bool {
	return !te.Soft && unusedExprMsg.MatchString(te.Msg) && !strings.HasSuffix(te.Msg, invalidOperandMsg)
}

// dupDeclMsg matches the messages of go/types for a name declared twice
// (go/types: decl.go, resolver.go, "F redeclared in this block" and its
// second error "\tother declaration of F", "method T.M already declared at
// ..."): Options.IgnoreDuplicateDecls may accept the package (dupdecls.go),
// the message of the error says so.
var dupDeclMsg = regexp.MustCompile(`^(\S+ redeclared in this block|\tother declaration of |method \S+ already declared at )`)

// isDupDeclErr reports whether err is a type error about a name declared
// twice.
func isDupDeclErr(err error) bool {
	var te types.Error
	return errors.As(err, &te) && dupDeclMsg.MatchString(te.Msg)
}

// listErrLine matches a line of a compiler error that go list reports:
// "./p.go:14:2: declared and not used: x".
var listErrLine = regexp.MustCompile(`^(.+):(\d+):(\d+): `)

// dropSoftErrors removes the type errors that pol ignores from pkgs and
// their dependencies (go/packages), and marks again as ill-typed only the
// packages that still have errors, or a dependency that has.
//
// go/packages keeps the errors of go/types in TypeErrors, and the same
// errors, as text, in Errors. go list also reports the errors of the
// compiler on the package as one error ("# path" and one line per error,
// with the messages of gc): it is removed when each of its lines is an
// ignored error (see softLines). The errors of go/types in TypeErrors are
// the gate: a hard error stays in Errors whatever the compiler error
// says.
func dropSoftErrors(pkgs []*packages.Package, pol typeErrPolicy) {
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		soft := map[[2]string]bool{} // position and message
		at := softLines{pol: pol}
		for _, te := range p.TypeErrors {
			if pol.ignores(te) {
				pos := te.Fset.Position(te.Pos)
				soft[[2]string{pos.String(), te.Msg}] = true
				at.add(pos.Filename, pos.Line, pos.Column)
			}
		}
		errs := p.Errors[:0]
		dropped := false // the last type error was dropped: drop its parts too
		for _, e := range p.Errors {
			switch {
			case e.Kind == packages.TypeError && isSubError(e.Msg) && dropped:
			case e.Kind == packages.TypeError && soft[[2]string{e.Pos, e.Msg}]:
				dropped = true
			case e.Kind == packages.ListError && at.all(e.Msg):
			default:
				if e.Kind == packages.TypeError {
					dropped = false
				}
				errs = append(errs, e)
			}
		}
		p.Errors = errs
		p.IllTyped = len(errs) > 0
		for _, imp := range p.Imports {
			if imp.IllTyped {
				p.IllTyped = true
			}
		}
	})
}

// softLines holds the positions of the ignored errors of go/types, to
// find the lines of a compiler error of go list that report them.
type softLines struct {
	pol    typeErrPolicy
	column map[string]bool // file base name:line:column
	line   map[string]bool // file base name:line
}

// add records the position of an ignored error.
func (s *softLines) add(file string, line, column int) {
	if s.column == nil {
		s.column, s.line = map[string]bool{}, map[string]bool{}
	}
	key := filepath.Base(file) + ":" + strconv.Itoa(line)
	s.line[key] = true
	s.column[key+":"+strconv.Itoa(column)] = true
}

// all reports whether msg is a compiler error of go list whose lines all
// report an ignored error: a line at the position of one, or on the same
// line as one with a message that the policy ignores (gc puts a slice
// expression "s[1:]" at its "[", go/types at "s").
func (s *softLines) all(msg string) bool {
	lines := strings.Split(strings.TrimSpace(msg), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "# ") {
		return false
	}
	for _, l := range lines[1:] {
		m := listErrLine.FindStringSubmatch(l)
		if m == nil {
			return false
		}
		key := filepath.Base(m[1]) + ":" + m[2]
		if s.column[key+":"+m[3]] {
			continue
		}
		if !s.line[key] || !s.pol.ignoresMsg(l[len(m[0]):]) {
			return false
		}
	}
	return true
}

// ignoresMsg reports whether msg, the message of a compiler error of gc
// (which words these errors as go/types does), is one that p ignores.
func (p typeErrPolicy) ignoresMsg(msg string) bool {
	return (p.unusedNames && unusedMsg.MatchString(msg)) ||
		(p.unusedExprs && isUnusedExprErr(types.Error{Msg: msg}))
}
