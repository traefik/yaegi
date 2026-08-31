package interp_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
)

func evalGorneshFlow(t *testing.T, i *interp.Interpreter, src string) string {
	t.Helper()
	v, err := i.Eval(src)
	if err != nil {
		t.Fatalf("Eval(%q): %v", src, err)
	}
	if !v.IsValid() {
		return ""
	}
	return fmt.Sprint(v.Interface())
}

func TestGorneshMixedMultiAssignmentCallPositions(t *testing.T) {
	tests := map[string]struct {
		assignment string
		want       string
	}{
		"first":  {assignment: "a, b, c = addHundred(a), b, c", want: "[101 2 3]"},
		"middle": {assignment: "a, b, c = a, addHundred(b), c", want: "[1 102 3]"},
		"last":   {assignment: "a, b, c = a, b, addHundred(c)", want: "[1 2 103]"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, "func addHundred(v int) int { return v + 100 }")
			got := evalGorneshFlow(t, i, "a, b, c := 1, 2, 3; "+tc.assignment+"; []int{a, b, c}")
			if got != tc.want {
				t.Fatalf("result = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGorneshMultiAssignmentPreservesParallelEvaluation(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshFlow(t, i, "func replace(p *int) int { *p = 99; return 7 }")

	got := evalGorneshFlow(t, i, "a, b := 1, 2; a, b = b, replace(&b); []int{a, b}")
	if got != "[2 7]" {
		t.Fatalf("earlier RHS was not snapshotted before later call: got %s, want [2 7]", got)
	}

	got = evalGorneshFlow(t, i, "c, d := 1, 2; c, d = replace(&c), c; []int{c, d}")
	if got != "[7 99]" {
		t.Fatalf("later RHS did not observe earlier call: got %s, want [7 99]", got)
	}

	evalGorneshFlow(t, i, "var boxed interface{}")
	got = evalGorneshFlow(t, i, "raw := 2; boxed, raw = raw, replace(&raw); []interface{}{boxed, raw}")
	if got != "[2 7]" {
		t.Fatalf("destination conversion was not preserved by snapshots: got %s, want [2 7]", got)
	}
}

func TestGorneshMultiAssignmentMapReceiverAndKeyResolution(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshFlow(t, i, `func replaceMapAndKey(target *map[int]int, key *int) int {
		*target = map[int]int{0: 10, 1: 20}
		*key = 1
		return 7
	}`)

	// Matches gc: a passive (variable) map receiver and key are resolved
	// when the assignment is applied, so a right-hand-side call that
	// rebinds either variable redirects the write.
	got := evalGorneshFlow(t, i, `oldMap := map[int]int{0: 1, 1: 2}
		currentMap := oldMap
		key := 0
		other := 0
		currentMap[key], other = 5, replaceMapAndKey(&currentMap, &key)
		[]int{oldMap[0], oldMap[1], currentMap[0], currentMap[1], key, other}`)
	if got != "[1 2 10 5 1 7]" {
		t.Fatalf("map receiver/key were not resolved at assignment time: got %s, want [1 2 10 5 1 7]", got)
	}

	// A non-passive key operand keeps its phase-1 evaluation order and
	// value: the call is evaluated before the right-hand side and its
	// result is retained even though the key variable moves.
	evalGorneshFlow(t, i, "func nextKey() int { return 9 }")
	got = evalGorneshFlow(t, i, `m := map[int]int{5: 42}
		n := 0
		m[nextKey()], n = 7, 2
		[]int{m[0], m[2], m[9], n}`)
	if got != "[0 0 7 2]" {
		t.Fatalf("non-passive map key was not snapshotted before RHS: got %s, want [0 0 7 2]", got)
	}
}

func TestGorneshMultiValueVarDeclarationsPreserveEvaluationOrder(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		i := interp.New(interp.Options{})
		evalGorneshFlow(t, i, "func replaceDecl(p *int) int { *p = 99; return 7 }")
		got := evalGorneshFlow(t, i, `(func() []int {
			prior := 2
			var first, second = prior, replaceDecl(&prior)
			return []int{first, second, prior}
		})()`)
		if got != "[2 7 99]" {
			t.Fatalf("local var declaration evaluation = %s, want [2 7 99]", got)
		}
	})

	t.Run("top level", func(t *testing.T) {
		i := interp.New(interp.Options{})
		evalGorneshFlow(t, i, "func replaceTopDecl(p *int) int { *p = 99; return 7 }")
		evalGorneshFlow(t, i, `var priorTopDecl = 2
			var firstTopDecl, secondTopDecl = priorTopDecl, replaceTopDecl(&priorTopDecl)`)
		got := evalGorneshFlow(t, i, "[]int{firstTopDecl, secondTopDecl, priorTopDecl}")
		if got != "[2 7 99]" {
			t.Fatalf("top-level var declaration evaluation = %s, want [2 7 99]", got)
		}
	})
}

func TestGorneshMultiAssignmentInForPostClause(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshFlow(t, i, "func addTenPost(v int) int { return v + 10 }")
	got := evalGorneshFlow(t, i, `iteration, value := 0, 1
		for ; iteration < 3; iteration, value = iteration + 1, addTenPost(value) {}
		[]int{iteration, value}`)
	if got != "[3 31]" {
		t.Fatalf("for-post multi-assignment = %s, want [3 31]", got)
	}
}

func TestGorneshMixedMultiAssignmentEvaluatesBlankRHS(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshFlow(t, i, "var blankSideEffect int")
	evalGorneshFlow(t, i, "func bumpBlank() int { blankSideEffect++; return blankSideEffect }")
	got := evalGorneshFlow(t, i, "x := 0; _, x = bumpBlank(), 7; []int{blankSideEffect, x}")
	if got != "[1 7]" {
		t.Fatalf("blank RHS/result = %s, want [1 7]", got)
	}
}

func TestGorneshTopLevelControlBodyMultiResultBindings(t *testing.T) {
	tests := map[string]struct {
		setup string
		body  string
		want  string
	}{
		"call": {
			setup: "func pairCall(v int) (int, error) { return v, nil }",
			body:  "value, err := pairCall(7); if err == nil { result = value }",
			want:  "7",
		},
		"map comma ok": {
			setup: "controlMap := map[string]int{\"answer\": 8}",
			body:  "value, ok := controlMap[\"answer\"]; if ok { result = value }",
			want:  "8",
		},
		"channel receive": {
			setup: "func makeControlChan() chan int { controlChan := make(chan int, 1); controlChan <- 9; return controlChan }",
			body:  "controlChan := makeControlChan(); value, ok := <-controlChan; if ok { result = value }",
			want:  "9",
		},
		"type assertion": {
			setup: "var controlAny interface{} = 10",
			body:  "value, ok := controlAny.(int); if ok { result = value }",
			want:  "10",
		},
		"var declaration": {
			setup: "func pairVar(v int) (int, error) { return v, nil }",
			body:  "var value, err = pairVar(11); if err == nil { result = value }",
			want:  "11",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, tc.setup)
			evalGorneshFlow(t, i, "result := 0; if true { "+tc.body+" }")
			if got := evalGorneshFlow(t, i, "result"); got != tc.want {
				t.Fatalf("result = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGorneshTopLevelMultiResultPersistsAcrossEvals(t *testing.T) {
	i := interp.New(interp.Options{})
	evalGorneshFlow(t, i, "func pairPersist() (int, int) { return 1, 2 }")
	evalGorneshFlow(t, i, "leftPersist, rightPersist := pairPersist()")
	for name, want := range map[string]int{"leftPersist": 1, "rightPersist": 2} {
		got, ok := i.Globals()[name]
		if !ok || got.Interface() != want {
			t.Fatalf("Globals()[%q] = %v (present=%v), want %d", name, got, ok, want)
		}
	}
	if got := evalGorneshFlow(t, i, "[]int{leftPersist, rightPersist}"); got != "[1 2]" {
		t.Fatalf("multi-result values did not persist: got %s, want [1 2]", got)
	}
}

func TestGorneshTopLevelLoopBodyMultiResultBindings(t *testing.T) {
	tests := map[string]struct {
		setup      string
		body       string
		resultName string
		want       string
		localNames []string
	}{
		"id and error": {
			setup:      "func loopID(v int) (int, error) { return v, nil }",
			body:       "id, err := loopID(15); if err == nil { idResult = id }",
			resultName: "idResult",
			want:       "15",
			localNames: []string{"id", "err"},
		},
		"content and error": {
			setup:      `func loopContent() (string, error) { return "file contents", nil }`,
			body:       "content, err := loopContent(); if err == nil { contentResult = content }",
			resultName: "contentResult",
			want:       "file contents",
			localNames: []string{"content", "err"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, tc.setup)
			if tc.resultName == "idResult" {
				evalGorneshFlow(t, i, "idResult := 0; for iteration := 0; iteration < 1; iteration++ { "+tc.body+" }")
			} else {
				evalGorneshFlow(t, i, `contentResult := ""; for iteration := 0; iteration < 1; iteration++ { `+tc.body+" }")
			}
			if got := evalGorneshFlow(t, i, tc.resultName); got != tc.want {
				t.Fatalf("%s = %q, want %q", tc.resultName, got, tc.want)
			}
			for _, localName := range tc.localNames {
				if _, err := i.Eval(localName); err == nil || !strings.Contains(err.Error(), "undefined: "+localName) {
					t.Fatalf("loop-body binding %q leaked or returned wrong error: %v", localName, err)
				}
			}
		})
	}
}

func TestGorneshImportedBinaryStringErrorBindingsInTopLevelLoops(t *testing.T) {
	tests := map[string]struct {
		exportName string
		export     reflect.Value
		call       string
		binding    string
		result     string
		want       string
	}{
		"id and error": {
			exportName: "Spawn",
			export: reflect.ValueOf(func(task, worker string) (string, error) {
				return "id-" + task, nil
			}),
			call:    `flow.Spawn(input, "worker")`,
			binding: "id",
			result:  "ids",
			want:    "[id-one id-two]",
		},
		"content and error": {
			exportName: "Read",
			export:     reflect.ValueOf(func(path string) (string, error) { return "content-" + path, nil }),
			call:       `flow.Read(input)`,
			binding:    "content",
			result:     "contents",
			want:       "[content-one content-two]",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			if err := i.Use(interp.Exports{"flow/flow": {
				tc.exportName: tc.export,
			}}); err != nil {
				t.Fatalf("Use binary flow package: %v", err)
			}
			evalGorneshFlow(t, i, `import "flow"`)
			evalGorneshFlow(t, i, tc.result+` := []string{}
				for _, input := range []string{"one", "two"} {
					`+tc.binding+`, err := `+tc.call+`
					if err != nil { panic(err) }
					`+tc.result+` = append(`+tc.result+`, `+tc.binding+`)
				}`)
			if got := evalGorneshFlow(t, i, tc.result); got != tc.want {
				t.Fatalf("%s = %s, want %s", tc.result, got, tc.want)
			}
			for _, localName := range []string{tc.binding, "err"} {
				if _, err := i.Eval(localName); err == nil || !strings.Contains(err.Error(), "undefined: "+localName) {
					t.Fatalf("loop-body binding %q leaked or returned wrong error: %v", localName, err)
				}
			}
		})
	}
}

func TestGorneshControlHeaderMultiResultBindingsDoNotLeak(t *testing.T) {
	tests := map[string]struct {
		control string
		want    string
	}{
		"if":     {control: "if value, err := headerPair(12); err == nil { result = value }", want: "12"},
		"for":    {control: "for value, err := headerPair(13); err == nil; { result = value; break }", want: "13"},
		"switch": {control: "switch value, err := headerPair(14); { case err == nil: result = value }", want: "14"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, "func headerPair(v int) (int, error) { return v, nil }")
			evalGorneshFlow(t, i, "result := 0; "+tc.control)
			if got := evalGorneshFlow(t, i, "result"); got != tc.want {
				t.Fatalf("control header result = %s, want %s", got, tc.want)
			}
			for _, name := range []string{"value", "err"} {
				if _, err := i.Eval(name); err == nil || !strings.Contains(err.Error(), "undefined: "+name) {
					t.Fatalf("header binding %q leaked or returned wrong error: %v", name, err)
				}
			}
		})
	}
}

func TestGorneshControlHeaderSingleResultBindingsDoNotLeak(t *testing.T) {
	tests := map[string]struct {
		control string
		want    string
		locals  []string
	}{
		"if": {
			control: "if header := 12; header == 12 { result = header }",
			want:    "12",
			locals:  []string{"header"},
		},
		"for": {
			control: "for header := 13; header == 13; { result = header; break }",
			want:    "13",
			locals:  []string{"header"},
		},
		"switch": {
			control: "switch header := 14; header { case 14: result = header }",
			want:    "14",
			locals:  []string{"header"},
		},
		"type switch": {
			control: `switch header := 10; typed := controlSubject.(type) {
				case int: result = header + typed
			}`,
			want:   "15",
			locals: []string{"header", "typed"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, "var controlSubject interface{} = 5")
			evalGorneshFlow(t, i, "result := 0; "+tc.control)
			if got := evalGorneshFlow(t, i, "result"); got != tc.want {
				t.Fatalf("control header result = %s, want %s", got, tc.want)
			}
			for _, localName := range tc.locals {
				if _, err := i.Eval(localName); err == nil || !strings.Contains(err.Error(), "undefined: "+localName) {
					t.Fatalf("header binding %q leaked or returned wrong error: %v", localName, err)
				}
			}
		})
	}
}

func TestGorneshControlHeaderBindingsShadowGlobals(t *testing.T) {
	tests := map[string]string{
		"if":          `if headerShadow := 1; true { _ = headerShadow }`,
		"for":         `for headerShadow := 1; headerShadow == 1; { break }`,
		"switch":      `switch headerShadow := 1; headerShadow { case 1: }`,
		"type switch": `switch headerShadow := 1; typedShadow := headerSubject.(type) { case int: _ = headerShadow + typedShadow }`,
	}
	for name, control := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, `var headerShadow = 100; var headerSubject interface{} = 5`)
			evalGorneshFlow(t, i, control)
			if got := evalGorneshFlow(t, i, `headerShadow`); got != "100" {
				t.Fatalf("control header overwrote global: got %s, want 100", got)
			}
			for _, localName := range []string{"typedShadow"} {
				if name != "type switch" {
					continue
				}
				if _, err := i.Eval(localName); err == nil || !strings.Contains(err.Error(), "undefined: "+localName) {
					t.Fatalf("type-switch binding %q leaked or returned wrong error: %v", localName, err)
				}
			}
		})
	}
}

func TestGorneshNestedTopLevelDeclarationsShadowGlobals(t *testing.T) {
	tests := map[string]string{
		"plain block short declaration": `{ nestedShadow := "local"; _ = nestedShadow }`,
		"if body var initializer":       `if true { var nestedShadow = "local"; _ = nestedShadow }`,
		"for body var without value":    `for once := true; once; once = false { var nestedShadow string; nestedShadow = "local" }`,
		"switch case short declaration": `switch 1 { case 1:
			nestedShadow := "local"
			_ = nestedShadow
		}`,
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			i := interp.New(interp.Options{})
			evalGorneshFlow(t, i, `var nestedShadow = 100`)
			evalGorneshFlow(t, i, src)
			if got := evalGorneshFlow(t, i, `nestedShadow`); got != "100" {
				t.Fatalf("nested declaration overwrote global: got %s, want 100", got)
			}
		})
	}
}
