package main

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/traefik/yaegi/internal/jsonx"
)

type writer struct {
	io.Writer
	Schema     *jsonx.Schema
	Name       string
	Embed      bool
	OmitEmpty  bool
	NoOptional bool

	seen       map[*jsonx.Schema]typedata
	seenPlain  map[jsonx.SimpleTypes]typedata
	properties map[string]map[string]bool
}

type kind int

const (
	otherType kind = iota
	primitiveType
	structType
	collectionType
)

type typedata struct {
	Name string
	Kind kind
	Type jsonx.SimpleTypes
}

func (w *writer) init() {
	w.seen = map[*jsonx.Schema]typedata{}
	w.seenPlain = map[jsonx.SimpleTypes]typedata{}
	w.properties = map[string]map[string]bool{}
}

func (w *writer) writeSchema(name string, s *jsonx.Schema) (typ typedata) {
	if typ := w.seen[s]; typ != (typedata{}) {
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
		return typedata{"interface{}", otherType, jsonx.SimpleTypes_Object}
	}

	if howMany(s.Enum, s.AllOf, s.AnyOf, s.OneOf, s.Not) > 1 {
		fatalf("type %q: enum, allOf, anyOf, oneOf, and not cannot be used together\n", name)
	}

	if s.AllOf != nil {
		return w.writeAllOf(name, s.AllOf)
	}

	if s.AnyOf != nil {
		fmt.Fprintf(os.Stderr, "type %q: anyOf not supported, using interface{}\n", name)
		return typedata{"interface{}", otherType, jsonx.SimpleTypes_Object}
	}

	if s.OneOf != nil {
		fmt.Fprintf(os.Stderr, "type %q: oneOf not supported, using interface{}\n", name)
		return typedata{"interface{}", otherType, jsonx.SimpleTypes_Object}
	}

	if s.Not != nil {
		fmt.Fprintf(os.Stderr, "type %q: not not supported, using interface{}\n", name)
		return typedata{"interface{}", otherType, jsonx.SimpleTypes_Object}
	}

	if s.Enum != nil {
		return w.writeEnum(name, s.Enum)
	}

	unsupported(name, s)
	panic("not reachable")
}

func (w *writer) writeRef(ref string) typedata {
	if len(ref) == 0 {
		fatalf("empty ref")
	}

	if ref[0] == '!' {
		return typedata{ref[1:], otherType, ""}
	}

	name, s := resolveRef(w.Schema, ref)
	if name == "" {
		return w.writeSchema(w.Name, s)
	}

	return w.writeSchema(camelCase(name), s)
}

func (w *writer) writeType(name string, s *jsonx.Schema) typedata {
	if len(s.Type) == 0 {
		// this is not actually valid according to the schema
		return typedata{"interface{}", otherType, jsonx.SimpleTypes_Object}
	}

	if isPlain(s) {
		if len(s.Type) > 1 {
			return typedata{"interface{}", otherType, jsonx.SimpleTypes_Object}
		}
		return w.writePlainType(s.Type[0])
	}

	if len(s.Type) > 1 {
		fatalf("type %q: unsupported: multiple types", name)
	}

	switch s.Type[0] {
	case jsonx.SimpleTypes_Object:
		return w.writeObjectType(name, s)

	case jsonx.SimpleTypes_Array:
		if s.AdditionalItems != nil {
			fatalf("type %q: additionalItems not supported\n", name)
		}
		el := w.writeSchema(name+"__Items", s.Items)
		if el.Kind == structType {
			return typedata{"[]*" + el.Name, collectionType, jsonx.SimpleTypes_Array}
		}
		return typedata{"[]" + el.Name, collectionType, jsonx.SimpleTypes_Array}

	case jsonx.SimpleTypes_Boolean:
		if isPlainExceptDefault(s) && s.Default == false {
			return w.writePlainType("boolean")
		}

	case jsonx.SimpleTypes_Integer:
		if isPlainExceptDefault(s) && s.Default == float64(0) {
			return w.writePlainType("integer")
		}

	case jsonx.SimpleTypes_Number:
		if isPlainExceptDefault(s) && s.Default == float64(0) {
			return w.writePlainType("number")
		}

	case jsonx.SimpleTypes_String:
		if isPlainExceptDefault(s) && s.Default == "" {
			return w.writePlainType("string")
		}
	}

	unsupported(name, s)
	panic("not reachable")
}

func (w *writer) writePlainType(name jsonx.SimpleTypes) (typ typedata) {
	if typ := w.seenPlain[name]; typ != (typedata{}) {
		return typ
	}
	defer func() { w.seenPlain[name] = typ }()

	switch name {
	case jsonx.SimpleTypes_Object:
		return typedata{"map[string]interface{}", collectionType, name}

	case jsonx.SimpleTypes_Array:
		return typedata{"[]interface{}", collectionType, name}

	case jsonx.SimpleTypes_Boolean:
		return typedata{"bool", primitiveType, name}

	case jsonx.SimpleTypes_Integer:
		return typedata{"int", primitiveType, name}

	case jsonx.SimpleTypes_Number:
		return typedata{"float64", primitiveType, name}

	case jsonx.SimpleTypes_String:
		return typedata{"string", primitiveType, name}

	default:
		panic(fmt.Sprintf("unsupported plain type %q", name))
	}
}

func (w *writer) writeNullableType(name jsonx.SimpleTypes) (typ typedata) {
	if typ := w.seenPlain[name+"?"]; typ != (typedata{}) {
		return typ
	}
	defer func() { w.seenPlain[name+"?"] = typ }()

	switch name {
	case jsonx.SimpleTypes_Boolean:
		fmt.Fprintf(w, "type Boolean bool\n")
		fmt.Fprintf(w, "func (v *Boolean) Eq(u bool) bool { return v != nil && bool(*v) == u }\n")
		fmt.Fprintf(w, "func (v *Boolean) Get() bool { return bool(*v) }\n")
		fmt.Fprintf(w, "func (v *Boolean) GetOr(u bool) bool { if v == nil { return u } else { return bool(*v) } }\n")
		fmt.Fprintf(w, "func (v *Boolean) True() bool { return v != nil && bool(*v) }\n")
		fmt.Fprintf(w, "func (v *Boolean) False() bool { return v != nil && !bool(*v) }\n")
		fmt.Fprintf(w, "\n")
		return typedata{"Boolean", primitiveType, name}

	case jsonx.SimpleTypes_Integer:
		fmt.Fprintf(w, "type Integer int\n")
		fmt.Fprintf(w, "func (v *Integer) Eq(u int) bool { return v != nil && int(*v) == u }\n")
		fmt.Fprintf(w, "func (v *Integer) Get() int { return int(*v) }\n")
		fmt.Fprintf(w, "func (v *Integer) GetOr(u int) int { if v == nil { return u } else { return int(*v) } }\n")
		fmt.Fprintf(w, "\n")
		return typedata{"Integer", primitiveType, name}

	case jsonx.SimpleTypes_Number:
		fmt.Fprintf(w, "type Number float64\n")
		fmt.Fprintf(w, "func (v *Number) Eq(u float64) bool { return v != nil && float64(*v) == u }\n")
		fmt.Fprintf(w, "func (v *Number) Get() float64 { return float64(*v) }\n")
		fmt.Fprintf(w, "func (v *Number) GetOr(u float64) float64 { if v == nil { return u } else { return float64(*v) } }\n")
		fmt.Fprintf(w, "\n")
		return typedata{"Number", primitiveType, name}

	case jsonx.SimpleTypes_String:
		fmt.Fprintf(w, "type String string\n")
		fmt.Fprintf(w, "func (v *String) Eq(u string) bool { return v != nil && string(*v) == u }\n")
		fmt.Fprintf(w, "func (v *String) Get() string { return string(*v) }\n")
		fmt.Fprintf(w, "func (v *String) GetOr(u string) string { if v == nil { return u } else { return string(*v) } }\n")
		fmt.Fprintf(w, "\n")
		return typedata{"String", primitiveType, name}

	default:
		panic(fmt.Sprintf("unsupported plain type %q", name))
	}
}

func (w *writer) writeObjectType(name string, s *jsonx.Schema) typedata {
	if m, ok := s.Default.(map[string]interface{}); ok && len(m) == 0 {
		// ok
	} else if s.Default != nil {
		fatalf("type %s: unsupported default: %v\n", name, s.Default)
	}

	switch {
	case s.AdditionalProperties == nil:
		w.seen[s] = typedata{name, structType, jsonx.SimpleTypes_Object}

		w.writeProperties(name, s)
		fmt.Fprintf(w, "\n")
		return typedata{name, structType, jsonx.SimpleTypes_Object}
	case s.Properties == nil:
		el := w.writeSchema(name+"__Values", s.AdditionalProperties)

		if el.Kind == structType {
			return typedata{"map[string]*" + el.Name, collectionType, jsonx.SimpleTypes_Object}
		}
		return typedata{"map[string]" + el.Name, collectionType, jsonx.SimpleTypes_Object}
	default:
		// TODO this needs a custom un/marshaller
		unsupported(name, s)
		panic("not reached")
	}
}

func (w *writer) writeProperties(name string, s *jsonx.Schema) {
	type Field struct {
		Name, Prop, Type string
		Order            int
	}
	fields := []Field{}
	embedded := map[string]bool{}
	for _, typ := range s.Embedded {
		fields = append(fields, Field{Type: typ})
		for prop := range w.properties[typ] {
			embedded[prop] = true
		}
	}

	required := map[string]bool{}
	for _, prop := range s.Required {
		required[prop] = true
	}

	w.properties[name] = map[string]bool{}
	for prop, s := range s.Properties {
		// skip fields that are an override of an embedded field
		if embedded[prop] {
			continue
		}

		w.properties[name][prop] = true

		fname := camelCase(prop)
		ftype := w.writeSchema(name+"_"+fname, s)

		var typ string
		switch {
		case w.NoOptional:
			typ = ftype.Name
			if ftype.Kind == structType {
				typ = "*" + typ
			}
		case required[prop]:
			typ = ftype.Name
		case ftype.Kind == primitiveType:
			ftype = w.writeNullableType(ftype.Type)
			typ = "*" + ftype.Name
		case ftype.Kind == structType:
			typ = "*" + ftype.Name
		default:
			typ = ftype.Name
		}

		fields = append(fields, Field{
			Prop:  prop,
			Name:  fname,
			Type:  typ,
			Order: s.Order,
		})
	}

	if w.OmitEmpty && len(fields) == len(s.Embedded) {
		return
	}

	sort.Slice(fields, func(i, j int) bool {
		fi, fj := fields[i], fields[j]
		ordered := fi.Name != "" && fj.Name != "" && (fi.Order > 0 || fj.Order > 0)
		switch {
		case ordered && fi.Order == 0:
			return false
		case ordered && fj.Order == 0:
			return true
		case ordered && fi.Order < fj.Order:
			return true
		case fi.Name < fj.Name:
			return true
		case fi.Name > fj.Name:
			return false
		default:
			return fi.Type < fj.Type
		}
	})

	fmt.Fprintf(w, "type %s struct {\n", name)
	for _, f := range fields {
		switch {
		case f.Name == "":
			fmt.Fprintf(w, "\t%s\n", f.Type)
		case required[f.Prop]:
			fmt.Fprintf(w, "\t%s %s `json:\"%s\"`\n", f.Name, f.Type, f.Prop)
		default:
			fmt.Fprintf(w, "\t%s %s `json:\"%s,omitempty\"`\n", f.Name, f.Type, f.Prop)
		}
	}
	fmt.Fprintf(w, "}\n")
}

func (w *writer) writeAllOf(name string, allOf []*jsonx.Schema) typedata {
	s := new(jsonx.Schema)

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
			s.Embedded = append(s.Embedded, typ.Name)
		}
	}

	return w.writeSchema(name, s)
}

func (w *writer) writeEnum(name string, values []string) typedata {
	name = camelCase(name)
	fmt.Fprintf(w, "type %s string\n", name)
	fmt.Fprintf(w, "const (\n")
	for _, v := range values {
		fmt.Fprintf(w, "\t%s_%s %s = %q\n", name, camelCase(v), name, v)
	}
	fmt.Fprintf(w, ")\n")
	fmt.Fprintf(w, "\n")
	return typedata{name, primitiveType, jsonx.SimpleTypes_String}
}
