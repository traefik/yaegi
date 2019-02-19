package interp_test

import (
	"testing"

	"github.com/containous/dyngo/interp"
)

func TestEval0(t *testing.T) {
	i := interp.New(interp.Opt{})
	_, err := i.Eval(`var I int = 2`)
	if err != nil {
		t.Fatal(err)
	}

	t1, err := i.Eval(`I`)
	if err != nil {
		t.Fatal(err)
	}
	if t1.Interface().(int) != 2 {
		t.Fatalf("expected 2, got %v", t1)
	}
}

func TestEval1(t *testing.T) {
	i := interp.New(interp.Opt{})
	_, err := i.Eval(`func Hello() string { return "hello" }`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := i.Eval(`Hello`)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := v.Interface().(func() string)
	if !ok {
		t.Fatal("conversion failed")
	}
	if s := f(); s != "hello" {
		t.Fatalf("expected hello, got %v", s)
	}
}

func TestEval2(t *testing.T) {
	i := interp.New(interp.Opt{})
	_, err := i.Eval(`package foo; var I int = 2`)
	if err != nil {
		t.Fatal(err)
	}

	t1, err := i.Eval(`foo.I`)
	if err != nil {
		t.Fatal(err)
	}
	if t1.Interface().(int) != 2 {
		t.Fatalf("expected 2, got %v", t1)
	}
}

func TestEval3(t *testing.T) {
	i := interp.New(interp.Opt{})
	_, err := i.Eval(`package foo; func Hello() string { return "hello" }`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := i.Eval(`foo.Hello`)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := v.Interface().(func() string)
	if !ok {
		t.Fatal("conversion failed")
	}
	if s := f(); s != "hello" {
		t.Fatalf("expected hello, got %v", s)
	}
}
