/*
Package extract generates wrappers of package exported symbols.
*/
package extract

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/constant"
	"go/format"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
)

// TODO(mpl): do we want to change that warning (s/goexports/something else/) ?
const model = `// Code generated by 'goexports {{.PkgName}}'. DO NOT EDIT.

{{.License}}

{{if .BuildTags}}// +build {{.BuildTags}}{{end}}

package {{.Dest}}

import (
{{- range $key, $value := .Imports }}
	{{- if $value}}
	"{{$key}}"
	{{- end}}
{{- end}}
	"{{.PkgName}}"
	"reflect"
)

func init() {
	Symbols["{{.PkgName}}"] = map[string]reflect.Value{
		{{- if .Val}}
		// function, constant and variable definitions
		{{range $key, $value := .Val -}}
			{{- if $value.Addr -}}
				"{{$key}}": reflect.ValueOf(&{{$value.Name}}).Elem(),
			{{else -}}
				"{{$key}}": reflect.ValueOf({{$value.Name}}),
			{{end -}}
		{{end}}

		{{- end}}
		{{- if .Typ}}
		// type definitions
		{{range $key, $value := .Typ -}}
			"{{$key}}": reflect.ValueOf((*{{$value}})(nil)),
		{{end}}

		{{- end}}
		{{- if .Wrap}}
		// interface wrapper definitions
		{{range $key, $value := .Wrap -}}
			"_{{$key}}": reflect.ValueOf((*{{$value.Name}})(nil)),
		{{end}}
		{{- end}}
	}
}
{{range $key, $value := .Wrap -}}
	// {{$value.Name}} is an interface wrapper for {{$key}} type
	type {{$value.Name}} struct {
		{{range $m := $value.Method -}}
		W{{$m.Name}} func{{$m.Param}} {{$m.Result}}
		{{end}}
	}
	{{range $m := $value.Method -}}
		func (W {{$value.Name}}) {{$m.Name}}{{$m.Param}} {{$m.Result}} { {{$m.Ret}} W.W{{$m.Name}}{{$m.Arg}} }
	{{end}}
{{end}}
`

// Val stores the value name and addressable status of symbols.
type Val struct {
	Name string // "package.name"
	Addr bool   // true if symbol is a Var
}

// Method stores information for generating interface wrapper method.
type Method struct {
	Name, Param, Result, Arg, Ret string
}

// Wrap stores information for generating interface wrapper.
type Wrap struct {
	Name   string
	Method []Method
}

// restricted map defines symbols for which a special implementation is provided.
var restricted = map[string]bool{
	"osExit":        true,
	"osFindProcess": true,
	"logFatal":      true,
	"logFatalf":     true,
	"logFatalln":    true,
	"logLogger":     true,
	"logNew":        true,
}

func genContent(dest, importPath, license string, p *types.Package, skip map[string]bool) ([]byte, error) {
	prefix := "_" + importPath + "_"
	prefix = strings.NewReplacer("/", "_", "-", "_", ".", "_").Replace(prefix)

	typ := map[string]string{}
	val := map[string]Val{}
	wrap := map[string]Wrap{}
	imports := map[string]bool{}
	sc := p.Scope()

	for _, pkg := range p.Imports() {
		imports[pkg.Path()] = false
	}
	qualify := func(pkg *types.Package) string {
		if pkg.Path() != importPath {
			imports[pkg.Path()] = true
		}
		return pkg.Name()
	}

	for _, name := range sc.Names() {
		o := sc.Lookup(name)
		if !o.Exported() {
			continue
		}

		pname := path.Base(importPath) + "." + name
		if skip[pname] {
			continue
		}

		if rname := path.Base(importPath) + name; restricted[rname] {
			// Restricted symbol, locally provided by stdlib wrapper.
			pname = rname
		}

		switch o := o.(type) {
		case *types.Const:
			if b, ok := o.Type().(*types.Basic); ok && (b.Info()&types.IsUntyped) != 0 {
				// convert untyped constant to right type to avoid overflow
				val[name] = Val{fixConst(pname, o.Val(), imports), false}
			} else {
				val[name] = Val{pname, false}
			}
		case *types.Func:
			val[name] = Val{pname, false}
		case *types.Var:
			val[name] = Val{pname, true}
		case *types.TypeName:
			typ[name] = pname
			if t, ok := o.Type().Underlying().(*types.Interface); ok {
				var methods []Method
				for i := 0; i < t.NumMethods(); i++ {
					f := t.Method(i)
					if !f.Exported() {
						continue
					}

					sign := f.Type().(*types.Signature)
					args := make([]string, sign.Params().Len())
					params := make([]string, len(args))
					for j := range args {
						v := sign.Params().At(j)
						if args[j] = v.Name(); args[j] == "" {
							args[j] = fmt.Sprintf("a%d", j)
						}
						params[j] = args[j] + " " + types.TypeString(v.Type(), qualify)
					}
					arg := "(" + strings.Join(args, ", ") + ")"
					param := "(" + strings.Join(params, ", ") + ")"

					results := make([]string, sign.Results().Len())
					for j := range results {
						v := sign.Results().At(j)
						results[j] = v.Name() + " " + types.TypeString(v.Type(), qualify)
					}
					result := "(" + strings.Join(results, ", ") + ")"

					ret := ""
					if sign.Results().Len() > 0 {
						ret = "return"
					}

					methods = append(methods, Method{f.Name(), param, result, arg, ret})
				}
				wrap[name] = Wrap{prefix + name, methods}
			}
		}
	}

	var buildTags string
	if runtime.Version() != "devel" {
		parts := strings.Split(runtime.Version(), ".")

		minorRaw := GetMinor(parts[1])

		currentGoVersion := parts[0] + "." + minorRaw

		minor, errParse := strconv.Atoi(minorRaw)
		if errParse != nil {
			return nil, fmt.Errorf("failed to parse version: %v", errParse)
		}

		nextGoVersion := parts[0] + "." + strconv.Itoa(minor+1)

		buildTags = currentGoVersion + ",!" + nextGoVersion
	}

	base := template.New("goexports")
	parse, err := base.Parse(model)
	if err != nil {
		return nil, fmt.Errorf("template parsing error: %v", err)
	}

	if importPath == "log/syslog" {
		buildTags += ",!windows,!nacl,!plan9"
	}

	b := new(bytes.Buffer)
	data := map[string]interface{}{
		"Dest":      dest,
		"Imports":   imports,
		"PkgName":   importPath,
		"Val":       val,
		"Typ":       typ,
		"Wrap":      wrap,
		"BuildTags": buildTags,
		"License":   license,
	}
	err = parse.Execute(b, data)
	if err != nil {
		return nil, fmt.Errorf("template error: %v", err)
	}

	// gofmt
	source, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to format source: %v: %s", err, b.Bytes())
	}
	return source, nil
}

// fixConst checks untyped constant value, converting it if necessary to avoid overflow.
func fixConst(name string, val constant.Value, imports map[string]bool) string {
	var (
		tok string
		str string
	)
	switch val.Kind() {
	case constant.Int:
		tok = "INT"
		str = val.ExactString()
	case constant.Float:
		v := constant.Val(val) // v is *big.Rat or *big.Float
		f, ok := v.(*big.Float)
		if !ok {
			f = new(big.Float).SetRat(v.(*big.Rat))
		}

		tok = "FLOAT"
		str = f.Text('g', int(f.Prec()))
	case constant.Complex:
		// TODO: not sure how to parse this case
		fallthrough
	default:
		return name
	}

	imports["go/constant"] = true
	imports["go/token"] = true

	return fmt.Sprintf("constant.MakeFromLiteral(\"%s\", token.%s, 0)", str, tok)
}

// importPath checks whether pkgIdent is an existing directory relative to
// e.WorkingDir. If yes, it returns the actual import path of the Go package
// located in the directory. If it is definitely a relative path, but it does not
// exist, an error is returned. Otherwise, it is assumed to be an import path, and
// pkgIdent is returned.
func (e Extractor) importPath(pkgIdent, importPath string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dirPath := filepath.Join(wd, pkgIdent)
	_, err = os.Stat(dirPath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err != nil {
		if len(pkgIdent) > 0 && pkgIdent[0] == '.' {
			// pkgIdent is definitely a relative path, not a package name, and it does not exist
			return "", err
		}
		// pkgIdent might be a valid stdlib package name. So we leave that responsibility to the caller now.
		return pkgIdent, nil
	}

	// local import
	if importPath != "" {
		return importPath, nil
	}

	modPath := filepath.Join(dirPath, "go.mod")
	_, err = os.Stat(modPath)
	if os.IsNotExist(err) {
		return "", errors.New("no go.mod found, and no import path specified")
	}
	if err != nil {
		return "", err
	}
	f, err := os.Open(modPath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()
	sc := bufio.NewScanner(f)
	var l string
	for sc.Scan() {
		l = sc.Text()
		break
	}
	if sc.Err() != nil {
		return "", err
	}
	parts := strings.Fields(l)
	if len(parts) < 2 {
		return "", errors.New(`invalid first line syntax in go.mod`)
	}
	if parts[0] != "module" {
		return "", errors.New(`invalid first line in go.mod, no "module" found`)
	}

	return parts[1], nil
}

// Extractor creates a package with all the symbols from a dependency package.
type Extractor struct {
	Dest    string // the name of the created package.
	License string // license text to be included in the created package, optional.
	Skip    map[string]bool
}

// Extract writes to rw a Go package with all the symbols found at pkgIdent.
// pkgIdent can be an import path, or a local path, relative to e.WorkingDir. In
// the latter case, Extract returns the actual import path of the package found at
// pkgIdent, otherwise it just returns pkgIdent.
// If pkgIdent is an import path, it is looked up in GOPATH. Vendoring is not
// supported yet, and the behavior is only defined for GO111MODULE=off.
func (e Extractor) Extract(pkgIdent, importPath string, rw io.Writer) (string, error) {
	ipp, err := e.importPath(pkgIdent, importPath)
	if err != nil {
		return "", err
	}

	pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(pkgIdent)
	if err != nil {
		return "", err
	}

	content, err := genContent(e.Dest, ipp, e.License, pkg, e.Skip)
	if err != nil {
		return "", err
	}

	if _, err := rw.Write(content); err != nil {
		return "", err
	}

	return ipp, nil
}

// GetMinor returns the minor part of the version number.
func GetMinor(part string) string {
	minor := part
	index := strings.Index(minor, "beta")
	if index < 0 {
		index = strings.Index(minor, "rc")
	}
	if index > 0 {
		minor = minor[:index]
	}

	return minor
}
