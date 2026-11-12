package interp_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
)

const callArgumentMutationGornesh = `
n := 0
inc := func() int { n++; return n }
`

type nativeReceiverSnapshotGornesh struct {
	value int
}

func (r nativeReceiverSnapshotGornesh) Read(int) int {
	return r.value
}

func nativeFirstMultiResultGornesh(int, int) int {
	return 1
}

func nativeSecondMultiResultGornesh(int, int) int {
	return 2
}

func nativePairAfterMutationGornesh(mutate func()) (int, int) {
	mutate()
	return 3, 4
}

func TestGorneshBuiltinCallArgumentsSnapshotLeftToRight(t *testing.T) {
	var stdout bytes.Buffer
	i := interp.New(interp.Options{Stdout: &stdout})
	evalGorneshGlobal(t, i, callArgumentMutationGornesh+`println(inc(), n, inc(), n)`)
	if got := strings.TrimSpace(stdout.String()); got != "1 1 2 2" {
		t.Fatalf("println arguments = %q, want %q", got, "1 1 2 2")
	}
}

func TestGorneshInterpretedCallArgumentsSnapshotLeftToRight(t *testing.T) {
	i := interp.New(interp.Options{})
	got := evalGorneshGlobal(t, i, callArgumentMutationGornesh+`
capture := func(a, b, c, d int) []int { return []int{a, b, c, d} }
capture(inc(), n, inc(), n)`).Interface()
	if want := []int{1, 1, 2, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("interpreted call arguments = %#v, want %#v", got, want)
	}
}

func TestGorneshNativeCallArgumentsSnapshotLeftToRight(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"snapshot/snapshot": {
		"Capture": reflect.ValueOf(func(a, b, c, d int) []int { return []int{a, b, c, d} }),
	}}); err != nil {
		t.Fatalf("Use native snapshot package: %v", err)
	}
	evalGorneshGlobal(t, i, `import "snapshot"`)
	got := evalGorneshGlobal(t, i, callArgumentMutationGornesh+`snapshot.Capture(inc(), n, inc(), n)`).Interface()
	if want := []int{1, 1, 2, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("native call arguments = %#v, want %#v", got, want)
	}
}

func TestGorneshCalledFunctionSnapshotsBeforeArguments(t *testing.T) {
	var stdout bytes.Buffer
	i := interp.New(interp.Options{Stdout: &stdout})
	evalGorneshGlobal(t, i, `
first := func(int) { println("first") }
second := func(int) { println("second") }
replace := func() int { first = second; return 1 }
first(replace())`)
	if got := strings.TrimSpace(stdout.String()); got != "first" {
		t.Fatalf("called function = %q, want %q", got, "first")
	}
}

func TestGorneshCalledFunctionSnapshotsBeforeMultiResultArgument(t *testing.T) {
	i := interp.New(interp.Options{})
	got := evalGorneshGlobal(t, i, `
first := func(int, int) int { return 1 }
second := func(int, int) int { return 2 }
chosen := first
pair := func() (int, int) { chosen = second; return 3, 4 }
chosen(pair())`).Interface()
	if got != 1 {
		t.Fatalf("interpreted target with multi-result argument = %#v, want 1", got)
	}
}

func TestGorneshDeferredFunctionSnapshotsBeforeMultiResultArgument(t *testing.T) {
	i := interp.New(interp.Options{})
	got := evalGorneshGlobal(t, i, `
result := 0
first := func(int, int) { result = 1 }
second := func(int, int) { result = 2 }
chosen := first
pair := func() (int, int) { chosen = second; return 3, 4 }
run := func() { defer chosen(pair()) }
run()
result`).Interface()
	if got != 1 {
		t.Fatalf("deferred target with multi-result argument = %#v, want 1", got)
	}
}

func TestGorneshNativeCalledFunctionSnapshotsBeforeMultiResultArgument(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"snapshot/snapshot": {
		"First":  reflect.ValueOf(nativeFirstMultiResultGornesh),
		"Second": reflect.ValueOf(nativeSecondMultiResultGornesh),
	}}); err != nil {
		t.Fatalf("Use native snapshot package: %v", err)
	}
	evalGorneshGlobal(t, i, `import "snapshot"`)
	got := evalGorneshGlobal(t, i, `
chosen := snapshot.First
pair := func() (int, int) { chosen = snapshot.Second; return 3, 4 }
chosen(pair())`).Interface()
	if got != 1 {
		t.Fatalf("native target with interpreted multi-result argument = %#v, want 1", got)
	}
}

func TestGorneshCalledFunctionSnapshotsBeforeNativeMultiResultArgument(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"snapshot/snapshot": {
		"PairAfterMutation": reflect.ValueOf(nativePairAfterMutationGornesh),
	}}); err != nil {
		t.Fatalf("Use native snapshot package: %v", err)
	}
	evalGorneshGlobal(t, i, `import "snapshot"`)
	got := evalGorneshGlobal(t, i, `
first := func(int, int) int { return 1 }
second := func(int, int) int { return 2 }
chosen := first
mutate := func() { chosen = second }
chosen(snapshot.PairAfterMutation(mutate))`).Interface()
	if got != 1 {
		t.Fatalf("interpreted target with native multi-result argument = %#v, want 1", got)
	}
}

func TestGorneshDeferredFunctionSnapshotsBeforeArguments(t *testing.T) {
	var stdout bytes.Buffer
	i := interp.New(interp.Options{Stdout: &stdout})
	evalGorneshGlobal(t, i, `
first := func(int) { println("first") }
second := func(int) { println("second") }
replace := func() int { first = second; return 1 }
run := func() { defer first(replace()) }
run()`)
	if got := strings.TrimSpace(stdout.String()); got != "first" {
		t.Fatalf("deferred function = %q, want %q", got, "first")
	}
}

func TestGorneshMethodReceiverSnapshotsBeforeArguments(t *testing.T) {
	i := interp.New(interp.Options{})
	got := evalGorneshGlobal(t, i, `
type receiverSnapshotGornesh struct { value int }
func (r receiverSnapshotGornesh) read(int) int { return r.value }
func (r *receiverSnapshotGornesh) readPointer(int) int { return r.value }
receiver := receiverSnapshotGornesh{value: 1}
replacement := receiverSnapshotGornesh{value: 2}
replace := func() int { receiver = replacement; return 0 }
valueResult := receiver.read(replace())
pointerReceiver := receiverSnapshotGornesh{value: 3}
mutatePointer := func() int { pointerReceiver.value = 4; return 0 }
pointerResult := pointerReceiver.readPointer(mutatePointer())
[]int{valueResult, pointerResult}`).Interface()
	if want := []int{1, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("method receivers = %#v, want %#v", got, want)
	}
}

func TestGorneshNativeMethodReceiverSnapshotsBeforeArguments(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"snapshot/snapshot": {
		"Receiver": reflect.ValueOf((*nativeReceiverSnapshotGornesh)(nil)),
		"New": reflect.ValueOf(func(value int) nativeReceiverSnapshotGornesh {
			return nativeReceiverSnapshotGornesh{value: value}
		}),
	}}); err != nil {
		t.Fatalf("Use native receiver package: %v", err)
	}
	evalGorneshGlobal(t, i, `import "snapshot"`)
	got := evalGorneshGlobal(t, i, `
receiver := snapshot.New(1)
replacement := snapshot.New(2)
replace := func() int { receiver = replacement; return 0 }
receiver.Read(replace())`).Interface()
	if got != 1 {
		t.Fatalf("native method receiver = %#v, want 1", got)
	}
}

func TestGorneshNonPrintBuiltinArgumentsSnapshotLeftToRight(t *testing.T) {
	i := interp.New(interp.Options{})
	got := evalGorneshGlobal(t, i, `
appendBase := []int{1}
appendOther := []int{9}
appendMutate := func() int { appendBase = appendOther; return 2 }
appendResult := append(appendBase, appendMutate())

copyFirst := []int{0}
copySecond := []int{0}
copyTarget := copyFirst
copySource := func() []int { copyTarget = copySecond; return []int{7} }
copy(copyTarget, copySource())

deleteFirst := map[string]int{"key": 1}
deleteSecond := map[string]int{"key": 2}
deleteTarget := deleteFirst
deleteKey := func() string { deleteTarget = deleteSecond; return "key" }
delete(deleteTarget, deleteKey())

length := 1
capacity := func() int { length = 3; return 4 }
made := make([]int, length, capacity())

realPart := 1.0
imagPart := func() float64 { realPart = 3; return 2 }
complexResult := complex(realPart, imagPart())

[]any{appendResult, copyFirst, copySecond, deleteFirst["key"], deleteSecond["key"], len(made), real(complexResult)}`).Interface()
	want := []any{[]int{1, 2}, []int{7}, []int{0}, 0, 2, 1, float64(1)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("builtin results = %#v, want %#v", got, want)
	}
}
