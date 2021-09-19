package main

import (
	"bytes"
	"go/format"
	"io/ioutil"
	"log"
	"strings"
	"text/template"
)

const model = `package interp

// Code generated by 'go run ../internal/cmd/genop/genop.go'. DO NOT EDIT.

import (
	"go/constant"
	"go/token"
	"reflect"
)

// Arithmetic operators
{{range $name, $op := .Arithmetic}}
func {{$name}}(n *node) {
	next := getExec(n.tnext)
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface
	dest := genValueOutput(n, typ)
	c0, c1 := n.child[0], n.child[1]

	switch typ.Kind() {
	{{- if $op.Str}}
	case reflect.String:
		switch {
		case isInterface:
			v0 := genValue(c0)
			v1 := genValue(c1)
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(v0(f).String() {{$op.Name}} v1(f).String()).Convert(typ))
				return next
			}
		case c0.rval.IsValid():
			s0 := vString(c0.rval)
			v1 := genValue(c1)
			n.exec = func(f *frame) bltn {
				dest(f).SetString(s0 {{$op.Name}} v1(f).String())
				return next
			}
		case c1.rval.IsValid():
			v0 := genValue(c0)
			s1 :=  vString(c1.rval)
			n.exec = func(f *frame) bltn {
				dest(f).SetString(v0(f).String() {{$op.Name}} s1)
				return next
			}
		default:
			v0 := genValue(c0)
			v1 := genValue(c1)
			n.exec = func(f *frame) bltn {
				dest(f).SetString(v0(f).String() {{$op.Name}} v1(f).String())
				return next
			}
		}
	{{- end}}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch {
		case isInterface:
			v0 := genValueInt(c0)
			{{- if $op.Shift}}
			v1 := genValueUint(c1)
			{{else}}
			v1 := genValueInt(c1)
			{{end -}}
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				_, j := v1(f)
				dest(f).Set(reflect.ValueOf(i {{$op.Name}} j).Convert(typ))
				return next
			}
		case c0.rval.IsValid():
			i := vInt(c0.rval)
			{{- if $op.Shift}}
			v1 := genValueUint(c1)
			{{else}}
			v1 := genValueInt(c1)
			{{end -}}
			n.exec = func(f *frame) bltn {
				_, j := v1(f)
				dest(f).SetInt(i {{$op.Name}} j)
				return next
			}
		case c1.rval.IsValid():
			v0 := genValueInt(c0)
			{{- if $op.Shift}}
			j := vUint(c1.rval)
			{{else}}
			j := vInt(c1.rval)
			{{end -}}
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				dest(f).SetInt(i {{$op.Name}} j)
				return next
			}
		default:
			v0 := genValueInt(c0)
			{{- if $op.Shift}}
			v1 := genValueUint(c1)
			{{else}}
			v1 := genValueInt(c1)
			{{end -}}
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				_, j := v1(f)
				dest(f).SetInt(i {{$op.Name}} j)
				return next
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch {
		case isInterface:
			v0 := genValueUint(c0)
			v1 := genValueUint(c1)
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				_, j := v1(f)
				dest(f).Set(reflect.ValueOf(i {{$op.Name}} j).Convert(typ))
				return next
			}
		case c0.rval.IsValid():
			i := vUint(c0.rval)
			v1 := genValueUint(c1)
			n.exec = func(f *frame) bltn {
				_, j := v1(f)
				dest(f).SetUint(i {{$op.Name}} j)
				return next
			}
		case c1.rval.IsValid():
			j := vUint(c1.rval)
			v0 := genValueUint(c0)
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				dest(f).SetUint(i {{$op.Name}} j)
				return next
			}
		default:
			v0 := genValueUint(c0)
			v1 := genValueUint(c1)
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				_, j := v1(f)
				dest(f).SetUint(i {{$op.Name}} j)
				return next
			}
		}
	{{- if $op.Float}}
	case reflect.Float32, reflect.Float64:
		switch {
		case isInterface:
			v0 := genValueFloat(c0)
			v1 := genValueFloat(c1)
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				_, j := v1(f)
				dest(f).Set(reflect.ValueOf(i {{$op.Name}} j).Convert(typ))
				return next
			}
		case c0.rval.IsValid():
			i := vFloat(c0.rval)
			v1 := genValueFloat(c1)
			n.exec = func(f *frame) bltn {
				_, j := v1(f)
				dest(f).SetFloat(i {{$op.Name}} j)
				return next
			}
		case c1.rval.IsValid():
			j := vFloat(c1.rval)
			v0 := genValueFloat(c0)
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				dest(f).SetFloat(i {{$op.Name}} j)
				return next
			}
		default:
			v0 := genValueFloat(c0)
			v1 := genValueFloat(c1)
			n.exec = func(f *frame) bltn {
				_, i := v0(f)
				_, j := v1(f)
				dest(f).SetFloat(i {{$op.Name}} j)
				return next
			}
		}
	case reflect.Complex64, reflect.Complex128:
		switch {
		case isInterface:
			v0 := genComplex(c0)
			v1 := genComplex(c1)
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(v0(f) {{$op.Name}} v1(f)).Convert(typ))
				return next
			}
		case c0.rval.IsValid():
			r0 := vComplex(c0.rval)
			v1 := genComplex(c1)
			n.exec = func(f *frame) bltn {
				dest(f).SetComplex(r0 {{$op.Name}} v1(f))
				return next
			}
		case c1.rval.IsValid():
			r1 := vComplex(c1.rval)
			v0 := genComplex(c0)
			n.exec = func(f *frame) bltn {
				dest(f).SetComplex(v0(f) {{$op.Name}} r1)
				return next
			}
		default:
			v0 := genComplex(c0)
			v1 := genComplex(c1)
			n.exec = func(f *frame) bltn {
				dest(f).SetComplex(v0(f) {{$op.Name}} v1(f))
				return next
			}
		}
	{{- end}}
	}
}

func {{$name}}Const(n *node) {
	v0, v1 := n.child[0].rval, n.child[1].rval
	{{- if $op.Shift}}
	isConst := (v0.IsValid() && isConstantValue(v0.Type()))
	{{- else}}
	isConst := (v0.IsValid() && isConstantValue(v0.Type())) && (v1.IsValid() && isConstantValue(v1.Type()))
	{{- end}}
	t := n.typ.rtype
	if isConst {
		t = constVal
	}
	n.rval = reflect.New(t).Elem()
	switch {
	case isConst:
		{{- if $op.Shift}}
		v := constant.Shift(vConstantValue(v0), token.{{tokenFromName $name}}, uint(vUint(v1)))
		n.rval.Set(reflect.ValueOf(v))
		{{- else if (eq $op.Name "/")}}
		var operator token.Token
		// When the result of the operation is expected to be an int (because both
		// operands are ints), we want to force the type of the whole expression to be an
		// int (and not a float), which is achieved by using the QUO_ASSIGN operator.
		if n.typ.untyped && isInt(n.typ.rtype) {
			operator = token.QUO_ASSIGN
		} else {
			operator = token.QUO
		}
		v := constant.BinaryOp(vConstantValue(v0), operator, vConstantValue(v1))
		n.rval.Set(reflect.ValueOf(v))
		{{- else}}
		{{- if $op.Int}}
		v := constant.BinaryOp(constant.ToInt(vConstantValue(v0)), token.{{tokenFromName $name}}, constant.ToInt(vConstantValue(v1)))
		{{- else}}
		v := constant.BinaryOp(vConstantValue(v0), token.{{tokenFromName $name}}, vConstantValue(v1))
		{{- end}}
		n.rval.Set(reflect.ValueOf(v))
		{{- end}}
	{{- if $op.Str}}
	case isString(t):
		n.rval.SetString(vString(v0) {{$op.Name}} vString(v1))
	{{- end}}
	{{- if $op.Float}}
	case isComplex(t):
		n.rval.SetComplex(vComplex(v0) {{$op.Name}} vComplex(v1))
	case isFloat(t):
		n.rval.SetFloat(vFloat(v0) {{$op.Name}} vFloat(v1))
	{{- end}}
	case isUint(t):
		n.rval.SetUint(vUint(v0) {{$op.Name}} vUint(v1))
	case isInt(t):
		{{- if $op.Shift}}
		n.rval.SetInt(vInt(v0) {{$op.Name}} vUint(v1))
		{{- else}}
		n.rval.SetInt(vInt(v0) {{$op.Name}} vInt(v1))
		{{- end}}
	}
}
{{end}}
// Assign operators
{{range $name, $op := .Arithmetic}}
func {{$name}}Assign(n *node) {
	next := getExec(n.tnext)
	typ := n.typ.TypeOf()
	c0, c1 := n.child[0], n.child[1]
	setMap := isMapEntry(c0)
	var mapValue, indexValue func(*frame) reflect.Value

	if setMap {
        mapValue = genValue(c0.child[0])
        indexValue = genValue(c0.child[1])
    }

	if c1.rval.IsValid() {
		switch typ.Kind() {
		{{- if $op.Str}}
		case reflect.String:
			v0 := genValueString(c0)
			v1 := vString(c1.rval)
			n.exec = func(f *frame) bltn {
				v, s := v0(f)
				v.SetString(s {{$op.Name}} v1)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		{{- end}}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v0 := genValueInt(c0)
			{{- if $op.Shift}}
			j := vUint(c1.rval)
			{{else}}
			j := vInt(c1.rval)
			{{end -}}
			n.exec = func(f *frame) bltn {
				v, i := v0(f)
				v.SetInt(i {{$op.Name}} j)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			v0 := genValueUint(c0)
			j := vUint(c1.rval)
			n.exec = func(f *frame) bltn {
				v, i := v0(f)
				v.SetUint(i {{$op.Name}} j)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		{{- if $op.Float}}
		case reflect.Float32, reflect.Float64:
			v0 := genValueFloat(c0)
			j := vFloat(c1.rval)
			n.exec = func(f *frame) bltn {
				v, i := v0(f)
				v.SetFloat(i {{$op.Name}} j)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		case reflect.Complex64, reflect.Complex128:
			v0 := genValue(c0)
			v1 := vComplex(c1.rval)
			n.exec = func(f *frame) bltn {
				v := v0(f)
				v.SetComplex(v.Complex() {{$op.Name}} v1)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		{{- end}}
		}
	} else {
		switch typ.Kind() {
		{{- if $op.Str}}
		case reflect.String:
			v0 := genValueString(c0)
			v1 := genValue(c1)
			n.exec = func(f *frame) bltn {
				v, s := v0(f)
				v.SetString(s {{$op.Name}} v1(f).String())
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		{{- end}}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v0 := genValueInt(c0)
			{{- if $op.Shift}}
			v1 := genValueUint(c1)
			{{else}}
			v1 := genValueInt(c1)
			{{end -}}
			n.exec = func(f *frame) bltn {
				v, i := v0(f)
				_, j := v1(f)
				v.SetInt(i {{$op.Name}} j)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			v0 := genValueUint(c0)
			v1 := genValueUint(c1)
			n.exec = func(f *frame) bltn {
				v, i := v0(f)
				_, j := v1(f)
				v.SetUint(i {{$op.Name}} j)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		{{- if $op.Float}}
		case reflect.Float32, reflect.Float64:
			v0 := genValueFloat(c0)
			v1 := genValueFloat(c1)
			n.exec = func(f *frame) bltn {
				v, i := v0(f)
				_, j := v1(f)
				v.SetFloat(i {{$op.Name}} j)
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		case reflect.Complex64, reflect.Complex128:
			v0 := genValue(c0)
			v1 := genValue(c1)
			n.exec = func(f *frame) bltn {
				v := v0(f)
				v.SetComplex(v.Complex() {{$op.Name}} v1(f).Complex())
				if setMap {
					mapValue(f).SetMapIndex(indexValue(f), v)
				}
				return next
			}
		{{- end}}
		}
	}
}
{{end}}
{{range $name, $op := .IncDec}}
func {{$name}}(n *node) {
	next := getExec(n.tnext)
	typ := n.typ.TypeOf()
	c0 := n.child[0]
	setMap := isMapEntry(c0)
	var mapValue, indexValue func(*frame) reflect.Value

	if setMap {
        mapValue = genValue(c0.child[0])
        indexValue = genValue(c0.child[1])
    }

	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v0 := genValueInt(c0)
		n.exec = func(f *frame) bltn {
			v, i := v0(f)
			v.SetInt(i {{$op.Name}} 1)
			if setMap {
                mapValue(f).SetMapIndex(indexValue(f), v)
            }
			return next
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v0 := genValueUint(c0)
		n.exec = func(f *frame) bltn {
			v, i := v0(f)
			v.SetUint(i {{$op.Name}} 1)
			if setMap {
                mapValue(f).SetMapIndex(indexValue(f), v)
            }
			return next
		}
	case reflect.Float32, reflect.Float64:
		v0 := genValueFloat(c0)
		n.exec = func(f *frame) bltn {
			v, i := v0(f)
			v.SetFloat(i {{$op.Name}} 1)
			if setMap {
                mapValue(f).SetMapIndex(indexValue(f), v)
            }
			return next
		}
	case reflect.Complex64, reflect.Complex128:
		v0 := genValue(c0)
		n.exec = func(f *frame) bltn {
			v := v0(f)
			v.SetComplex(v.Complex() {{$op.Name}} 1)
			if setMap {
                mapValue(f).SetMapIndex(indexValue(f), v)
            }
			return next
		}
	}
}
{{end}}
{{range $name, $op := .Unary}}
func {{$name}}Const(n *node) {
	v0 := n.child[0].rval
	isConst := v0.IsValid() && isConstantValue(v0.Type())
	t := n.typ.rtype
	if isConst {
		t = constVal
	}
	n.rval = reflect.New(t).Elem()

	{{- if $op.Bool}}
	if isConst {
		v := constant.UnaryOp(token.{{tokenFromName $name}}, vConstantValue(v0), 0)
		n.rval.Set(reflect.ValueOf(v))
	} else {
		n.rval.SetBool({{$op.Name}} v0.Bool())
	}
	{{- else}}
	switch {
	case isConst:
		v := constant.UnaryOp(token.{{tokenFromName $name}}, vConstantValue(v0), 0)
		n.rval.Set(reflect.ValueOf(v))
	case isUint(t):
		n.rval.SetUint({{$op.Name}} v0.Uint())
	case isInt(t):
		n.rval.SetInt({{$op.Name}} v0.Int())
	{{- if $op.Float}}
	case isFloat(t):
		n.rval.SetFloat({{$op.Name}} v0.Float())
	case isComplex(t):
		n.rval.SetComplex({{$op.Name}} v0.Complex())
	{{- end}}
	}
	{{- end}}
}
{{end}}
{{range $name, $op := .Comparison}}
func {{$name}}(n *node) {
	tnext := getExec(n.tnext)
	dest := genValueOutput(n, reflect.TypeOf(true))
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface
	c0, c1 := n.child[0], n.child[1]

	{{- if or (eq $op.Name "==") (eq $op.Name "!=") }}

	if c0.typ.cat == aliasT || c1.typ.cat == aliasT {
		switch {
		case isInterface:
			v0 := genValue(c0)
			v1 := genValue(c1)
			dest := genValue(n)
			n.exec = func(f *frame) bltn {
				i0 := v0(f).Interface()
				i1 := v1(f).Interface()
				dest(f).Set(reflect.ValueOf(i0 {{$op.Name}} i1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			i0 := c0.rval.Interface()
			v1 := genValue(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					i1 := v1(f).Interface()
					if i0 != i1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					i1 := v1(f).Interface()
					dest(f).SetBool(i0 {{$op.Name}} i1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			i1 := c1.rval.Interface()
			v0 := genValue(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					if i0 != i1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					dest(f).SetBool(i0 {{$op.Name}} i1)
					return tnext
				}
			}
		default:
			v0 := genValue(c0)
			v1 := genValue(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					i1 := v1(f).Interface()
					if i0 != i1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					i1 := v1(f).Interface()
					dest(f).SetBool(i0 {{$op.Name}} i1)
					return tnext
				}
			}
		}
		return
	}
	{{- end}}

	switch t0, t1 := c0.typ.TypeOf(), c1.typ.TypeOf(); {
	case isString(t0) || isString(t1):
		switch {
		case isInterface:
			v0 := genValueString(c0)
			v1 := genValueString(c1)
			n.exec = func(f *frame) bltn {
				_, s0 := v0(f)
				_, s1 := v1(f)
				dest(f).Set(reflect.ValueOf(s0 {{$op.Name}} s1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			s0 :=  vString(c0.rval)
			v1 := genValueString(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			s1 :=  vString(c1.rval)
			v0 := genValueString(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		default:
			v0 := genValueString(c0)
			v1 := genValueString(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		}
	case isFloat(t0) || isFloat(t1):
		switch {
		case isInterface:
			v0 := genValueFloat(c0)
			v1 := genValueFloat(c1)
			n.exec = func(f *frame) bltn {
				_, s0 := v0(f)
				_, s1 := v1(f)
				dest(f).Set(reflect.ValueOf(s0 {{$op.Name}} s1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			s0 := vFloat(c0.rval)
			v1 := genValueFloat(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			s1 := vFloat(c1.rval)
			v0 := genValueFloat(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		default:
			v0 := genValueFloat(c0)
			v1 := genValueFloat(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		}
	case isUint(t0) || isUint(t1):
		switch {
		case isInterface:
			v0 := genValueUint(c0)
			v1 := genValueUint(c1)
			n.exec = func(f *frame) bltn {
				_, s0 := v0(f)
				_, s1 := v1(f)
				dest(f).Set(reflect.ValueOf(s0 {{$op.Name}} s1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			s0 := vUint(c0.rval)
			v1 := genValueUint(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			s1 := vUint(c1.rval)
			v0 := genValueUint(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		default:
			v0 := genValueUint(c0)
			v1 := genValueUint(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		}
	case isInt(t0) || isInt(t1):
		switch {
		case isInterface:
			v0 := genValueInt(c0)
			v1 := genValueInt(c1)
			n.exec = func(f *frame) bltn {
				_, s0 := v0(f)
				_, s1 := v1(f)
				dest(f).Set(reflect.ValueOf(s0 {{$op.Name}} s1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			s0 := vInt(c0.rval)
			v1 := genValueInt(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			s1 := vInt(c1.rval)
			v0 := genValueInt(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		default:
			v0 := genValueInt(c0)
			v1 := genValueInt(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					_, s0 := v0(f)
					_, s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		}
	{{- if $op.Complex}}
	case isComplex(t0) || isComplex(t1):
		switch {
		case isInterface:
			v0 := genComplex(c0)
			v1 := genComplex(c1)
			n.exec = func(f *frame) bltn {
				s0 := v0(f)
				s1 := v1(f)
				dest(f).Set(reflect.ValueOf(s0 {{$op.Name}} s1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			s0 := vComplex(c0.rval)
			v1 := genComplex(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				n.exec = func(f *frame) bltn {
					s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			s1 := vComplex(c1.rval)
			v0 := genComplex(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					s0 := v0(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					s0 := v0(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		default:
			v0 := genComplex(c0)
			v1 := genComplex(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					s0 := v0(f)
					s1 := v1(f)
					if s0 {{$op.Name}} s1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				n.exec = func(f *frame) bltn {
					s0 := v0(f)
					s1 := v1(f)
					dest(f).SetBool(s0 {{$op.Name}} s1)
					return tnext
				}
			}
		}
	default:
		switch {
		case isInterface:
			v0 := genValue(c0)
			v1 := genValue(c1)
			n.exec = func(f *frame) bltn {
				i0 := v0(f).Interface()
				i1 := v1(f).Interface()
				dest(f).Set(reflect.ValueOf(i0 {{$op.Name}} i1).Convert(typ))
				return tnext
			}
		case c0.rval.IsValid():
			i0 := c0.rval.Interface()
			v1 := genValue(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					i1 := v1(f).Interface()
					if i0 {{$op.Name}} i1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					i1 := v1(f).Interface()
					dest(f).SetBool(i0 {{$op.Name}} i1)
					return tnext
				}
			}
		case c1.rval.IsValid():
			i1 := c1.rval.Interface()
			v0 := genValue(c0)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					if i0 {{$op.Name}} i1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					dest(f).SetBool(i0 {{$op.Name}} i1)
					return tnext
				}
			}
		default:
			v0 := genValue(c0)
			v1 := genValue(c1)
			if n.fnext != nil {
				fnext := getExec(n.fnext)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					i1 := v1(f).Interface()
					if i0 {{$op.Name}} i1 {
						dest(f).SetBool(true)
						return tnext
					}
					dest(f).SetBool(false)
					return fnext
				}
			} else {
				dest := genValue(n)
				n.exec = func(f *frame) bltn {
					i0 := v0(f).Interface()
					i1 := v1(f).Interface()
					dest(f).SetBool(i0 {{$op.Name}} i1)
					return tnext
				}
			}
		}
	{{- end}}
	}
}
{{end}}
`

// Op define operator name and properties.
type Op struct {
	Name    string // +, -, ...
	Str     bool   // true if operator applies to string
	Float   bool   // true if operator applies to float
	Complex bool   // true if operator applies to complex
	Shift   bool   // true if operator is a shift operation
	Bool    bool   // true if operator applies to bool
	Int     bool   // true if operator applies to int only
}

func main() {
	base := template.New("genop")
	base.Funcs(template.FuncMap{
		"tokenFromName": func(name string) string {
			switch name {
			case "andNot":
				return "AND_NOT"
			case "neg":
				return "SUB"
			case "pos":
				return "ADD"
			case "bitNot":
				return "XOR"
			default:
				return strings.ToUpper(name)
			}
		},
	})
	parse, err := base.Parse(model)
	if err != nil {
		log.Fatal(err)
	}

	b := &bytes.Buffer{}
	data := map[string]interface{}{
		"Arithmetic": map[string]Op{
			"add":    {"+", true, true, true, false, false, false},
			"sub":    {"-", false, true, true, false, false, false},
			"mul":    {"*", false, true, true, false, false, false},
			"quo":    {"/", false, true, true, false, false, false},
			"rem":    {"%", false, false, false, false, false, true},
			"shl":    {"<<", false, false, false, true, false, true},
			"shr":    {">>", false, false, false, true, false, true},
			"and":    {"&", false, false, false, false, false, true},
			"or":     {"|", false, false, false, false, false, true},
			"xor":    {"^", false, false, false, false, false, true},
			"andNot": {"&^", false, false, false, false, false, true},
		},
		"IncDec": map[string]Op{
			"inc": {Name: "+"},
			"dec": {Name: "-"},
		},
		"Comparison": map[string]Op{
			"equal":        {Name: "==", Complex: true},
			"greater":      {Name: ">", Complex: false},
			"greaterEqual": {Name: ">=", Complex: false},
			"lower":        {Name: "<", Complex: false},
			"lowerEqual":   {Name: "<=", Complex: false},
			"notEqual":     {Name: "!=", Complex: true},
		},
		"Unary": map[string]Op{
			"not":    {Name: "!", Float: false, Bool: true},
			"neg":    {Name: "-", Float: true, Bool: false},
			"pos":    {Name: "+", Float: true, Bool: false},
			"bitNot": {Name: "^", Float: false, Bool: false, Int: true},
		},
	}
	if err = parse.Execute(b, data); err != nil {
		log.Fatal(err)
	}

	// gofmt
	source, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	if err = ioutil.WriteFile("op.go", source, 0o666); err != nil {
		log.Fatal(err)
	}
}
