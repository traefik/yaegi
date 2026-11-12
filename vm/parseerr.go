package vm

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"

	"golang.org/x/tools/go/packages"
)

// parseFile parses a Go file for the loaders that use go/packages
// (Config.ParseFile), with the syntax errors that gc would report.
//
// go/packages parses with parser.AllErrors, which keeps every error of
// go/parser: after a bad token, the parser also reports the tokens it
// expected there ("expected declaration, found 'ILLEGAL'", "expected ';',
// found int"). gc reports the bad character only. Without AllErrors,
// go/parser drops the errors that follow another error on the same line
// ("likely a spurious error") and stops after 10 errors, as gc does. The
// errors of go/scanner are all kept.
//
// go/scanner also reports a bad character twice: once when it reads it
// ("illegal character NUL", "illegal UTF-8 encoding"), and once when it
// makes an ILLEGAL token of it ("illegal character U+0000", "illegal
// character U+FFFD"). It already skips the second error for a byte order
// mark; scanErrors drops it for the two others. The GOROOT test nul1.go
// checks the messages of these lines.
func parseFile(fset *token.FileSet, name string, src []byte) (*ast.File, error) {
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	return f, scanErrors(err)
}

// scanErrors drops from the errors of go/parser the second error of
// go/scanner for a NUL character or for bytes that are not UTF-8.
func scanErrors(err error) error {
	var list scanner.ErrorList
	if !errors.As(err, &list) {
		return err
	}
	at := map[token.Position]int{} // errors at each position
	for _, e := range list {
		at[e.Pos]++
	}
	out := list[:0:0]
	for _, e := range list {
		if at[e.Pos] > 1 && (e.Msg == "illegal character U+0000" || e.Msg == "illegal character U+FFFD '�'") {
			at[e.Pos]--
			continue
		}
		out = append(out, e)
	}
	return out.Err()
}

// dropTypeErrorsAfterSyntax removes the type errors of the packages of pkgs
// (and of their dependencies) that have syntax errors: go/packages type
// checks what go/parser could read, and the type errors there are often
// follow-on errors of the syntax errors. gc does not type check a package
// after a parser error ("syntax error: ..."). After a lexical error only (a
// NUL character, bytes that are not UTF-8), gc skips the bad bytes and type
// checks the rest; go/scanner cannot do that: it makes an ILLEGAL token of
// them, which breaks the declaration around it, and gives type errors that
// gc does not ("undefined: A" for "var \xc2A int", where gc reads
// "var A int"). So the VM reports no type error after any syntax error. A
// type error that gc would also report after a lexical error is then left
// out: the program is refused all the same.
//
// A package that imports a package with syntax errors gets no type errors
// either: gc does not compile it (the build stops at the dependency), and
// the declarations that the syntax error broke give errors in the importer
// that gc does not report ("undefined: v.G" when the syntax error made the
// parser lose G). The importer is refused all the same, by the syntax
// errors of its dependency.
func dropTypeErrorsAfterSyntax(pkgs []*packages.Package) {
	syntax := map[*packages.Package]bool{} // has syntax errors, or imports a package that has
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		// Visit calls this after the imports of p: syntax[imp] is known.
		for _, e := range p.Errors {
			if e.Kind == packages.ParseError {
				syntax[p] = true
				break
			}
		}
		for _, imp := range p.Imports {
			if syntax[imp] {
				syntax[p] = true
				break
			}
		}
		if !syntax[p] {
			return
		}
		errs := p.Errors[:0]
		for _, e := range p.Errors {
			if e.Kind != packages.TypeError {
				errs = append(errs, e)
			}
		}
		p.Errors = errs
	})
}
