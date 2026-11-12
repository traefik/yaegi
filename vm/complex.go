package vm

// Complex numbers. A complex64/complex128 value is Value{R: complex128}; the
// zero Value (R == nil) is 0. complex64 results are rounded to float32
// components. Complex operations are rare: they are OpFunc closures.

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

func isComplex(t types.Type) bool {
	b := basic(t)
	return b != nil && b.Info()&types.IsComplex != 0
}

// cplx returns the complex payload of v.
func cplx(v Value) complex128 {
	c, _ := v.R.(complex128)
	return c
}

// cplxVal builds a value of complex type t (rounding for complex64).
func cplxVal(c complex128, t types.Type) Value {
	if b := basic(t); b != nil && b.Kind() == types.Complex64 {
		c = complex128(complex64(c))
	}
	return Value{R: c}
}

func constComplex(v constant.Value) complex128 {
	re, _ := constant.Float64Val(constant.ToFloat(constant.Real(v)))
	im, _ := constant.Float64Val(constant.ToFloat(constant.Imag(v)))
	return complex(re, im)
}

// cmul is a*b with each product rounded to float64 before the sum: the
// compiler may fuse a*c - b*d into one fused multiply-add (it does on
// arm64), which gives other last digits, or NaN against +Inf. The VM never
// fuses, as its scalar float ops, so the result is the same on every
// platform.
func cmul(a, b complex128) complex128 {
	ar, ai, br, bi := real(a), imag(a), real(b), imag(b)
	return complex(float64(ar*br)-float64(ai*bi), float64(ar*bi)+float64(ai*br))
}

// complexBinop compiles a binary operation on complex operands.
func (c *compiler) complexBinop(v *ssa.BinOp, dst, x, y int32) {
	t := v.Type()
	var f func(a, b complex128) Value
	switch v.Op {
	case token.ADD:
		f = func(a, b complex128) Value { return cplxVal(a+b, t) }
	case token.SUB:
		f = func(a, b complex128) Value { return cplxVal(a-b, t) }
	case token.MUL:
		f = func(a, b complex128) Value { return cplxVal(cmul(a, b), t) }
	case token.QUO:
		f = func(a, b complex128) Value { return cplxVal(a/b, t) }
	case token.EQL:
		f = func(a, b complex128) Value { return Value{N: b2i(a == b)} }
	case token.NEQ:
		f = func(a, b complex128) Value { return Value{N: b2i(a != b)} }
	default:
		unsupported("binop %s on %s", v.Op, v.X.Type())
	}
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return f(cplx(regs[x]), cplx(regs[y])) }})
}

// complexBuiltin compiles complex, real and imag.
func (c *compiler) complexBuiltin(name string, dst int32, args []ssa.Value, rt types.Type) {
	rs := c.regList(args)
	switch name {
	case "complex":
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
			return cplxVal(complex(f64(regs[rs[0]]), f64(regs[rs[1]])), rt)
		}})
	case "real":
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return Value{N: fv(real(cplx(regs[rs[0]])))} }})
	case "imag":
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return Value{N: fv(imag(cplx(regs[rs[0]])))} }})
	}
}
