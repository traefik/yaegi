package interp

import (
	"go/constant"
	"log"
	"reflect"
	"testing"

	"github.com/traefik/yaegi/stdlib"
)

func init() { log.SetFlags(log.Lshortfile) }

func TestIsNatural(t *testing.T) {
	tests := []struct {
		desc     string
		n        *node
		expected bool
	}{
		{
			desc: "positive uint var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var a uint = 3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					var a uint = 3
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive untyped var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						a := 3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					a := 3
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive int var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var a int = 3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					var a int = 3
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive float var, null decimal",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var a float64 = 3.0
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					var a float64 = 3.0
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive float var, with decimal",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var a float64 = 3.14
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					var a float64 = 3.14
					return reflect.ValueOf(a)
				}(),
			},
			expected: false,
		},
		{
			desc: "negative int var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var a int = -3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					var a int = -3
					return reflect.ValueOf(a)
				}(),
			},
			expected: false,
		},
		{
			desc: "positive typed const",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						const a uint = 3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					const a uint = 3
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive untyped const",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						const a = 3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					const a = 3
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive untyped const (iota)",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						const (
							zero = iota
							a
						)
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					const (
						zero = iota
						a
					)
					return reflect.ValueOf(a)
				}(),
			},
			expected: true,
		},
		{
			desc: "negative const",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						const a = -3
						return reflect.TypeOf(a)
					}(),
				},
				rval: func() reflect.Value {
					const a = -3
					return reflect.ValueOf(a)
				}(),
			},
			expected: false,
		},
	}
	for _, test := range tests {
		got := test.n.isNatural()
		if test.expected != got {
			t.Fatalf("%s: got %v, wanted %v", test.desc, got, test.expected)
		}
	}
}

func TestGlobals(t *testing.T) {
	i := New(Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval("var a = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval("b := 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval("const c = 3"); err != nil {
		t.Fatal(err)
	}

	g := i.Globals()
	a := g["a"]
	if !a.IsValid() {
		t.Fatal("a not found")
	}
	if a := a.Interface(); a != 1 {
		t.Fatalf("wrong a: want (%[1]T) %[1]v, have (%[2]T) %[2]v", 1, a)
	}
	b := g["b"]
	if !b.IsValid() {
		t.Fatal("b not found")
	}
	if b := b.Interface(); b != 2 {
		t.Fatalf("wrong b: want (%[1]T) %[1]v, have (%[2]T) %[2]v", 2, b)
	}
	c := g["c"]
	if !c.IsValid() {
		t.Fatal("c not found")
	}
	if cc, ok := c.Interface().(constant.Value); ok && constant.MakeInt64(3) != cc {
		t.Fatalf("wrong c: want (%[1]T) %[1]v, have (%[2]T) %[2]v", constant.MakeInt64(3), cc)
	}
}
