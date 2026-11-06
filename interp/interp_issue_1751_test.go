package interp_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// Issue #1751: a negative signed shift count must panic at run time, matching Go.
func TestNegativeShiftCountPanics(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "fmt"`); err != nil {
		t.Fatal(err)
	}

	src := `
func shl1751(x uint64, n int8) (r uint64, err any) {
	defer func() { err = recover() }()
	return x << n, nil
}

func shr1751(x int32, n int) (r int32, err any) {
	defer func() { err = recover() }()
	return x >> n, nil
}
`
	if _, err := i.Eval(src); err != nil {
		t.Fatal(err)
	}

	out, err := i.Eval(`fmt.Sprint(shl1751(1, -1))`)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%v", out)
	if !strings.Contains(got, "0") || !strings.Contains(got, "runtime error: negative shift amount") {
		t.Fatalf("shl1751(1,-1) = %q, want zero + recovered negative shift panic", got)
	}

	out, err = i.Eval(`fmt.Sprint(shr1751(-8, -3))`)
	if err != nil {
		t.Fatal(err)
	}
	got = fmt.Sprintf("%v", out)
	if !strings.Contains(got, "0") || !strings.Contains(got, "runtime error: negative shift amount") {
		t.Fatalf("shr1751(-8,-3) = %q, want zero + recovered negative shift panic", got)
	}

	_, err = i.Eval(`n1751 := -1; _ = 1 << n1751`)
	if err == nil {
		t.Fatal("expected panic for unrecovered negative shift")
	}
	if !strings.Contains(err.Error(), "negative shift amount") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNonNegativeShiftStillWorks(t *testing.T) {
	i := interp.New(interp.Options{})
	cases := []struct {
		src, res string
	}{
		{`1 << 3`, `8`},
		{`16 >> 2`, `4`},
		{`a, b := 1, 3; a << b`, `8`},
		{`c, d := 16, 2; c >> d`, `4`},
		{`g := 1; g <<= 3; g`, `8`},
		{`h := 16; h >>= 2; h`, `4`},
		{`u := uint8(1); uint64(1) << u`, `2`},
	}
	for _, c := range cases {
		v, err := i.Eval(c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		got := fmt.Sprintf("%v", v)
		if got != c.res {
			t.Fatalf("%s = %q, want %q", c.src, got, c.res)
		}
	}
}

// Mutation guard: if the negative check is removed, this fails.
func TestNegativeShiftAssignPanics(t *testing.T) {
	i := interp.New(interp.Options{})
	_, err := i.Eval(`x := 1; n := -1; x <<= n`)
	if err == nil {
		t.Fatal("expected panic for negative <<= count")
	}
	if !strings.Contains(err.Error(), "negative shift amount") {
		t.Fatalf("unexpected error: %v", err)
	}
}
