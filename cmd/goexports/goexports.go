//go:generate go build

/*
Goexports generates wrappers of package exported symbols

Output files are written in the current directory, and prefixed with the go version.

Usage:

    goexports package...

Example:

    goexports github.com/containous/yaegi/interp

The same goexport program is used for all target operating systems and architectures.
The GOOS and GOARCH environment variables set the desired target.
*/
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/constant"
	"go/format"
	"go/importer"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"text/template"
)

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

// Val store the value name and addressable status of symbols
type Val struct {
	Name string // "package.name"
	Addr bool   // true if symbol is a Var
}

// Method store information for generating interface wrapper method
type Method struct {
	Name, Param, Result, Arg, Ret string
}

// Wrap store information for generating interface wrapper
type Wrap struct {
	Name   string
	Method []Method
}

func genContent(dest, pkgName, license string) ([]byte, error) {
	p, err := importer.For("source", nil).Import(pkgName)
	if err != nil {
		return nil, err
	}

	prefix := "_" + pkgName + "_"
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
		if pkg.Path() != pkgName {
			imports[pkg.Path()] = true
		}
		return pkg.Name()
	}

	for _, name := range sc.Names() {
		o := sc.Lookup(name)
		if !o.Exported() {
			continue
		}

		pname := path.Base(pkgName) + "." + name
		switch o := o.(type) {
		case *types.Const:
			val[name] = Val{fixConst(pname, o.Val()), false}
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

		minorRaw := getMinor(parts[1])

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

	if pkgName == "log/syslog" {
		buildTags += ",!windows,!nacl,!plan9"
	}

	b := new(bytes.Buffer)
	data := map[string]interface{}{
		"Dest":      dest,
		"Imports":   imports,
		"PkgName":   pkgName,
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

// fixConst checks untyped constant value, converting it if necessary to avoid overflow
func fixConst(name string, val constant.Value) string {
	if val.Kind() == constant.Int {
		str := val.ExactString()
		i, err := strconv.ParseInt(str, 0, 64)
		if err == nil {
			switch {
			case i == int64(int32(i)):
				return name
			case i == int64(uint32(i)):
				return "uint32(" + name + ")"
			default:
				return "int64(" + name + ")"
			}
		}
		_, err = strconv.ParseUint(str, 0, 64)
		if err == nil {
			return "uint64(" + name + ")"
		}
	}
	return name
}

// genLicense generates the correct LICENSE header text from the provided
// path to a LICENSE file.
func genLicense(fname string) (string, error) {
	if fname == "" {
		return "", nil
	}

	f, err := os.Open(fname)
	if err != nil {
		return "", fmt.Errorf("could not open LICENSE file: %v", err)
	}
	defer func() { _ = f.Close() }()

	license := new(strings.Builder)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		txt := sc.Text()
		if txt != "" {
			txt = " " + txt
		}
		license.WriteString("//" + txt + "\n")
	}
	if sc.Err() != nil {
		return "", fmt.Errorf("could not scan LICENSE file: %v", err)
	}

	return license.String(), nil
}

func main() {
	licenseFlag := flag.String("license", "", "path to a LICENSE file")

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		log.Fatalf("missing package path")
	}

	license, err := genLicense(*licenseFlag)
	if err != nil {
		log.Fatal(err)
	}

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	dest := path.Base(dir)

	for _, pkg := range flag.Args() {
		content, err := genContent(dest, pkg, license)
		if err != nil {
			log.Fatal(err)
		}

		var oFile string
		if pkg == "syscall" {
			goos, arch := os.Getenv("GOOS"), os.Getenv("GOARCH")
			oFile = strings.Replace(pkg, "/", "_", -1) + "_" + goos + "_" + arch + ".go"
		} else {
			oFile = strings.Replace(pkg, "/", "_", -1) + ".go"
		}

		prefix := runtime.Version()
		if runtime.Version() != "devel" {
			parts := strings.Split(runtime.Version(), ".")

			prefix = parts[0] + "_" + getMinor(parts[1])
		}

		err = ioutil.WriteFile(prefix+"_"+oFile, content, 0666)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getMinor(part string) string {
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
