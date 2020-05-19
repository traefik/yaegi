package interp

import (
	"log"
	"reflect"
	"testing"
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
						var x uint = 3
						return reflect.TypeOf(x)
					}(),
				},
				rval: func() reflect.Value {
					var x uint = 3
					return reflect.ValueOf(x)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive untyped var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var x = 3
						return reflect.TypeOf(x)
					}(),
				},
				rval: func() reflect.Value {
					var x = 3
					return reflect.ValueOf(x)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive int var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var x int = 3
						return reflect.TypeOf(x)
					}(),
				},
				rval: func() reflect.Value {
					var x int = 3
					return reflect.ValueOf(x)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive float var, null decimal",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var x float64 = 3.0
						return reflect.TypeOf(x)
					}(),
				},
				rval: func() reflect.Value {
					var x float64 = 3.0
					return reflect.ValueOf(x)
				}(),
			},
			expected: true,
		},
		{
			desc: "positive float var, with decimal",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var x float64 = 3.14
						return reflect.TypeOf(x)
					}(),
				},
				rval: func() reflect.Value {
					var x float64 = 3.14
					return reflect.ValueOf(x)
				}(),
			},
			expected: false,
		},
		{
			desc: "negative int var",
			n: &node{
				typ: &itype{
					rtype: func() reflect.Type {
						var x int = -3
						return reflect.TypeOf(x)
					}(),
				},
				rval: func() reflect.Value {
					var x int = -3
					return reflect.ValueOf(x)
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
