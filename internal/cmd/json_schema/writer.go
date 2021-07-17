package main

import (
	"fmt"
	"io"
	"os"
	"sort"
)

type writer struct {
	io.Writer
	Schema *Schema
	Name   string
	Embed  bool

	seen       map[*Schema]string
	seenPlain  map[SimpleTypes]string
	properties map[string]map[string]bool
}

func (w *writer) init() {
	w.seen = map[*Schema]string{}
	w.seenPlain = map[SimpleTypes]string{}
	w.properties = map[string]map[string]bool{}
}

func (w *writer) writeSchema(name string, s *Schema) (typ string) {
	if typ := w.seen[s]; typ != "" {
		return typ
	}
	defer func() {
		w.seen[s] = typ
	}()

	if s.PatternProperties != nil {
		fatalf("type %q: pattern properties are not supported\n", name)
	}

	if s.Ref != "" {
		if !isPlain(s) {
			fatalf("type %q: non-plain ref types are not supported\n", name)
		}
		return w.writeRef(s.Ref)
	}

	if s.Type != nil && s.Enum == nil {
		return w.writeType(name, s)
	}

	if isPlain(s) {
		return "interface{}"
	}

	if howMany(s.Enum, s.AllOf, s.AnyOf, s.OneOf, s.Not) > 1 {
		fatalf("type %q: enum, allOf, anyOf, oneOf, and not cannot be used together\n", name)
	}

	if s.AllOf != nil {
		return w.writeAllOf(name, s.AllOf)
	}

	if s.AnyOf != nil {
		fmt.Fprintf(os.Stderr, "type %q: anyOf not supported, using interface{}\n", name)
		return "interface{}"
	}

	if s.OneOf != nil {
		fmt.Fprintf(os.Stderr, "type %q: oneOf not supported, using interface{}\n", name)
		return "interface{}"
	}

	if s.Not != nil {
		fmt.Fprintf(os.Stderr, "type %q: not not supported, using interface{}\n", name)
		return "interface{}"
	}

	if s.Enum != nil {
		return w.writeEnum(name, s.Enum)
	}

	unsupported(name, s)
	panic("not reachable")
}

func (w *writer) writeRef(ref string) string {
	if len(ref) == 0 {
		fatalf("empty ref")
	}

	if ref[0] == '!' {
		return ref[1:]
	}

	name, s := resolveRef(w.Schema, ref)
	if name == "" {
		return w.writeSchema(w.Name, s)
	}

	return w.writeSchema(camelCase(name), s)
}

func (w *writer) writeType(name string, s *Schema) string {
	if len(s.Type) == 0 {
		// this is not actually valid according to the schema
		return "interface{}"
	}

	if isPlain(s) {
		if len(s.Type) > 1 {
			return "interface{}"
		}
		return w.writePlainType(s.Type[0])
	}

	if len(s.Type) > 1 {
		fatalf("type %q: unsupported: multiple types", name)
	}

	switch s.Type[0] {
	case SimpleTypes_Object:
		return w.writeObjectType(name, s)

	case SimpleTypes_Array:
		if s.AdditionalItems != nil {
			fatalf("type %q: additionalItems not supported\n", name)
		}
		return "[]" + w.writeSchema(name+"__Items", s.Items)

	case SimpleTypes_Boolean:
		if isPlainExceptDefault(s) && s.Default == false {
			return w.writePlainType("boolean")
		}

	case SimpleTypes_Integer:
		if isPlainExceptDefault(s) && s.Default == float64(0) {
			return w.writePlainType("integer")
		}

	case SimpleTypes_Number:
		if isPlainExceptDefault(s) && s.Default == float64(0) {
			return w.writePlainType("number")
		}

	case SimpleTypes_String:
		if isPlainExceptDefault(s) && s.Default == "" {
			return w.writePlainType("string")
		}
	}

	unsupported(name, s)
	panic("not reachable")
}

func (w *writer) writePlainType(name SimpleTypes) (typ string) {
	if typ := w.seenPlain[name]; typ != "" {
		return typ
	}
	defer func() { w.seenPlain[name] = typ }()

	switch name {
	case SimpleTypes_Object:
		return "map[string]interface{}"

	case SimpleTypes_Array:
		return "[]interface{}"

	case SimpleTypes_Boolean:
		return "bool"

	case SimpleTypes_Integer:
		return "int"

	case SimpleTypes_Number:
		return "float64"

	case SimpleTypes_String:
		return "string"

	default:
		panic(fmt.Sprintf("unsupported plain type %q", name))
	}
}

func (w *writer) writeObjectType(name string, s *Schema) string {
	if m, ok := s.Default.(map[string]interface{}); ok && len(m) == 0 {
		// ok
	} else if s.Default != nil {
		fatalf("type %s: unsupported default: %v\n", name, s.Default)
	}

	var typ string
	if s.AdditionalProperties == nil {
		typ = "*" + name
		w.seen[s] = typ

		w.writeProperties(name, s)
		fmt.Fprintf(w, "\n")
		return typ
	} else if s.Properties == nil {
		return "map[string]" + w.writeSchema(name+"__Values", s.AdditionalProperties)
	} else {
		// TODO this needs a custom un/marshaller
		unsupported(name, s)
		panic("not reached")
	}
}

func (w *writer) writeProperties(name string, s *Schema) {
	type Field struct{ Name, Prop, Type string }
	fields := []Field{}
	w.properties[name] = map[string]bool{}
	for prop, s := range s.Properties {
		w.properties[name][prop] = true

		var f Field
		f.Prop = prop
		if prop[0] == '$' {
			prop = prop[1:]
		}

		f.Name = camelCase(prop)
		f.Type = w.writeSchema(name+"_"+f.Name, s)
		fields = append(fields, f)
	}

	for _, typ := range s.Embedded {
		fields = append(fields, Field{Type: typ})
	}

	sort.Slice(fields, func(i, j int) bool {
		switch {
		case fields[i].Name < fields[j].Name:
			return true
		case fields[i].Name > fields[j].Name:
			return false
		default:
			return fields[i].Type < fields[j].Type
		}
	})

	fmt.Fprintf(w, "type %s struct {\n", name)
	for _, f := range fields {
		if f.Name == "" {
			fmt.Fprintf(w, "\t%s\n", f.Type)
		} else {
			fmt.Fprintf(w, "\t%s %s `json:\"%s,omitempty\"`\n", f.Name, f.Type, f.Prop)
		}
	}
	fmt.Fprintf(w, "}\n")
}

func (w *writer) writeAllOf(name string, allOf []*Schema) string {
	s := new(Schema)

	opts := mergeOpts{
		Base:        w.Schema,
		Recurse:     true,
		ResolveRefs: !w.Embed,
	}

	for i, r := range allOf {
		if opts.ResolveRefs || r.Ref == "" {
			schemaMerge(opts, fmt.Sprintf("%s[%d]", name, i), s, r)
		} else {
			typ := w.writeSchema(resolveRef(w.Schema, r.Ref))
			if typ[0] == '*' {
				typ = typ[1:]
			}
			s.Embedded = append(s.Embedded, typ)
		}
	}

	for _, typ := range s.Embedded {
		for prop := range w.properties[typ] {
			delete(s.Properties, prop)
		}
	}

	return w.writeSchema(name, s)
}

func (w *writer) writeEnum(name string, values []string) string {
	name = camelCase(name)
	fmt.Fprintf(w, "type %s string\n", name)
	fmt.Fprintf(w, "const(\n")
	for _, v := range values {
		fmt.Fprintf(w, "\t%s_%s %s = %q\n", name, camelCase(v), name, v)
	}
	fmt.Fprintf(w, ")\n")
	fmt.Fprintf(w, "\n")
	return name
}
