package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/traefik/yaegi/internal/jsonx"
)

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

var schemaUrl = flag.String("url", "", "URL of the schema")
var packageName = flag.String("name", "", "Package name")
var filePath = flag.String("path", "", "File to write to")
var jsonPatch = flag.String("patch", "", "JSON file to apply as a patch")
var chooseTypes = flag.String("choose", "", "Comma-separated list of types to extract, instead of all types")
var verbose = flag.Bool("verbose", false, "Print more messages")

var dapMode = flag.Bool("dap-mode", false, "Used for generating Debug Adapter Protocol types")

func main() {
	flag.Parse()

	if *schemaUrl == "" {
		fatalf("--url is required")
	}
	if *packageName == "" {
		fatalf("--name is required")
	}
	if *filePath == "" {
		fatalf("--path is required")
	}

	resp, err := http.Get(*schemaUrl)
	if err != nil {
		fatalf("http: %v\n", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		fatalf("http: expected status 200, got %d\n", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		fatalf("read (schema): %v\n", err)
	}

	if *jsonPatch != "" {
		b, err = jsonx.ApplyPatch(b, *jsonPatch)
		if err != nil {
			fatalf("%v\n", err)
		}
	}

	var schema Schema
	err = json.Unmarshal(b, &schema)
	if err != nil {
		fatalf("json (schema): %v\n", err)
	}

	buf := new(bytes.Buffer)
	w := &writer{
		Writer:     buf,
		Schema:     &schema,
		Name:       "Schema",
		Embed:      *dapMode,
		OmitEmpty:  *dapMode,
		NoOptional: !*dapMode,
	}
	w.init()

	fmt.Fprintf(w, "package %s\n", *packageName)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "// Code generated by 'go run ../internal/cmd/genschema'. DO NOT EDIT.\n")
	fmt.Fprintf(w, "\n")

	var m map[string]bool
	if *chooseTypes != "" {
		m = map[string]bool{}
		for _, typ := range strings.Split(*chooseTypes, ",") {
			m[typ] = true
		}
	}

	forEachOrdered(w.Schema.Definitions, func(name string, s *Schema) {
		name = camelCase(name)
		if m == nil || m[name] {
			w.writeSchema(name, s)
		} else if *verbose {
			fmt.Fprintf(os.Stderr, "Omitting %s\n", name)
		}
	})

	if w.Schema.Properties != nil {
		w.writeSchema(w.Name, w.Schema)
	}

	f, err := os.Create(*filePath)
	if err != nil {
		fatalf("json: %v\n", err)
	}
	defer f.Close()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *filePath, buf, parser.ParseComments)
	if err != nil {
		fatalf("json: %v\n", err)
	}

	err = format.Node(f, fset, file)
	if err != nil {
		fatalf("json: %v\n", err)
	}
}

func isPlain(s *Schema) bool {
	return s.Default == nil && isPlainExceptDefault(s)
}

func isPlainExceptDefault(s *Schema) bool {
	return true &&
		s.Items == nil &&
		s.AdditionalProperties == nil &&
		s.Definitions == nil &&
		s.Properties == nil &&
		s.PatternProperties == nil &&
		s.Enum == nil &&
		s.AllOf == nil &&
		s.AnyOf == nil &&
		s.OneOf == nil &&
		s.Not == nil
}

func howMany(v ...interface{}) int {
	c := 0
	for _, v := range v {
		if v != nil && !reflect.ValueOf(v).IsNil() {
			c++
		}
	}
	return c
}

func resolveRef(s *Schema, ref string) (string, *Schema) {
	if ref == "#" {
		return "", s
	}

	const schemaDefs = "#/definitions/"
	if strings.HasPrefix(ref, schemaDefs) {
		name := ref[len(schemaDefs):]
		s, ok := s.Definitions[name]
		if !ok {
			fatalf("missing definition for %q\n", name)
		}
		return name, s
	}

	fatalf("unsupported ref %q\n", ref)
	panic("not reachable")
}

func forEachOrdered(m map[string]*Schema, fn func(string, *Schema)) {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}

	sort.StringSlice(names).Sort()

	for _, name := range names {
		fn(name, m[name])
	}
}

func unsupported(name string, s *Schema) {
	fmt.Fprintf(os.Stderr, "type %q: unsupported schema\n", name)
	enc := json.NewEncoder(os.Stderr)
	enc.SetIndent("", "    ")
	enc.Encode(s)
	os.Exit(1)
}

type mergeOpts struct {
	Base        *Schema
	ResolveRefs bool
	Recurse     bool
}

func schemaMerge(opts mergeOpts, name string, s, r *Schema) {
	if opts.ResolveRefs && r.Ref != "" {
		if !isPlain(r) {
			fatalf("type %q: non-plain ref types are not supported\n", name)
		}
		_, r = resolveRef(opts.Base, r.Ref)
	}

	if opts.Recurse && r.AllOf != nil {
		s := new(Schema)
		for i, r := range r.AllOf {
			schemaMerge(opts, fmt.Sprintf("%s[%d]", name, i), s, r)
		}
		r = s
	}

	if s.Description == "" {
		s.Description = r.Description
	} else if r.Description != "" {
		s.Description += "\n" + r.Description
	}

	schemaReplaceField(opts, name, s, r, "Default")
	schemaReplaceField(opts, name, s, r, "AdditionalItems")
	schemaReplaceField(opts, name, s, r, "Items")
	schemaReplaceField(opts, name, s, r, "Required")
	schemaReplaceField(opts, name, s, r, "AdditionalProperties")
	schemaReplaceField(opts, name, s, r, "Definitions")
	schemaReplaceField(opts, name, s, r, "Properties")
	schemaReplaceField(opts, name, s, r, "PatternProperties")
	schemaReplaceField(opts, name, s, r, "Dependencies")
	schemaReplaceField(opts, name, s, r, "Enum")
	schemaReplaceField(opts, name, s, r, "Type")
	schemaReplaceField(opts, name, s, r, "AllOf")
	schemaReplaceField(opts, name, s, r, "AnyOf")
	schemaReplaceField(opts, name, s, r, "OneOf")
	schemaReplaceField(opts, name, s, r, "Not")
}

func schemaReplaceField(opts mergeOpts, name string, s, r *Schema, field string) {
	rf := reflect.ValueOf(r).Elem().FieldByName(field)
	rv := rf.Interface()
	if rf.IsNil() {
		return
	}

	sf := reflect.ValueOf(s).Elem().FieldByName(field)
	sv := sf.Interface()
	if sf.IsNil() {
		if _, ok := sv.(map[string]*Schema); ok {
			sv = map[string]*Schema{}
			sf.Set(reflect.ValueOf(sv))
		} else {
			sf.Set(rf)
			return
		}
	}

	switch sv := sv.(type) {
	case Schema_Type:
		m := map[SimpleTypes]bool{}
		for _, v := range rv.(Schema_Type) {
			m[v] = true
		}
		nv := Schema_Type{}
		for _, v := range sv {
			if m[v] {
				nv = append(nv, v)
			}
		}
		sf.Set(reflect.ValueOf(nv))

	case map[string]*Schema:
		for k, v := range rv.(map[string]*Schema) {
			if sv[k] == nil {
				sv[k] = v
			} else {
				schemaMerge(opts, name+"["+k+"]", sv[k], v)
			}
		}

	default:
		if !reflect.DeepEqual(sv, rv) && field != "Enum" {
			fatalf("type %q: unsupported operation: attempting to overwrite field %s\n", name, field)
		}
	}
}

func camelCase(s string) string {
	sb := new(strings.Builder)
	sb.Grow(len(s))

	upcase := true
	for _, r := range s {
		switch {
		case r == '$':
			// ignore
		case r == '_':
			upcase = true
		case upcase:
			sb.WriteRune(unicode.ToTitle(r))
			upcase = false
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}