package interp

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func TestGorneshCanceledSourceImportRetriesPreparedInitialization(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var calls atomic.Int32
	i := New(Options{
		GoPath: "gopath",
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/retryinit/retryinit.go": {Data: []byte(`package retryinit
import "retryhost"
var V = retryhost.Value()
`)},
		},
	})
	if err := i.Use(Exports{"retryhost/retryhost": {
		"Value": reflect.ValueOf(func() int {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
			return 42
		}),
	}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `import "retryinit"; retryinit.V`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("source import initializer did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled source import error = %v, want context canceled", err)
	}

	value, err := i.Eval(`import "retryinit"; retryinit.V`)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("retried source import: value=%v err=%v", value, err)
	}
	releaseOnce.Do(func() { close(release) })
	// Serialize behind the abandoned worker's final compileMu reacquisition.
	value, err = i.Eval(`retryinit.V`)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("committed source import after abandoned worker exit: value=%v err=%v", value, err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("source initializer calls = %d, want one canceled and one committed attempt", got)
	}
}

func TestGorneshCanceledNestedSourceImportDoesNotLeaveFalseCycle(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var calls atomic.Int32
	i := New(Options{
		GoPath: "gopath",
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/nestedinner/inner.go": {Data: []byte(`package nestedinner
import "nestedhost"
var V = nestedhost.Value()
`)},
			"gopath/src/nestedouter/outer.go": {Data: []byte(`package nestedouter
import "nestedinner"
var V = nestedinner.V
`)},
		},
	})
	if err := i.Use(Exports{"nestedhost/nestedhost": {
		"Value": reflect.ValueOf(func() int {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
			return 42
		}),
	}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `import "nestedouter"; nestedouter.V`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("nested source initializer did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled nested source import error = %v, want context canceled", err)
	}

	value, err := i.Eval(`import "nestedouter"; nestedouter.V`)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("retried nested source import: value=%v err=%v", value, err)
	}
	releaseOnce.Do(func() { close(release) })
	value, err = i.Eval(`nestedouter.V`)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("committed nested source import after abandoned worker exit: value=%v err=%v", value, err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("nested initializer calls = %d, want one canceled and one committed attempt", got)
	}
}

func TestGorneshCanceledSourceDirectoryRetriesInitializationAndMain(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var mainCalls atomic.Int32
	i := New(Options{
		GoPath: "gopath",
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/retryapp/main.go": {Data: []byte(`package main
import "retrydirhost"
var V = retrydirhost.Value()
func main() { retrydirhost.MarkMain() }
`)},
		},
	})
	if err := i.Use(Exports{"retrydirhost/retrydirhost": {
		"Value": reflect.ValueOf(func() int {
			entered <- struct{}{}
			<-release
			return 42
		}),
		"MarkMain": reflect.ValueOf(func() { mainCalls.Add(1) }),
	}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := i.EvalPathWithContext(ctx, "retryapp")
		first <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("source-directory initializer did not enter native call")
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled source-directory error = %v, want context canceled", err)
	}

	retry := make(chan error, 1)
	go func() {
		_, err := i.EvalPath("retryapp")
		retry <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("source-directory retry did not rerun initializer")
	}
	select {
	case err := <-retry:
		t.Fatalf("source-directory retry returned before initializer release: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-retry; err != nil {
		t.Fatalf("source-directory retry: %v", err)
	}
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("serialize after source-directory retry: %v", err)
	}
	if got := mainCalls.Load(); got != 1 {
		t.Fatalf("source-directory main calls = %d, want only committed retry", got)
	}
	symbol := i.Symbols("retryapp")["retryapp"]["V"]
	if !symbol.IsValid() || symbol.Interface() != 42 {
		t.Fatalf("retried source-directory V = %v, want 42", symbol)
	}
}

func TestGorneshPanickingSourceInitializerIsRetryableFailure(t *testing.T) {
	var calls atomic.Int32
	i := New(Options{
		GoPath: "gopath",
		Stderr: io.Discard,
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/retrypanic/retrypanic.go": {Data: []byte(`package retrypanic
import "retrypanichost"
var V = func() int {
	retrypanichost.Mark()
	panic("boom")
}()
`)},
		},
	})
	if err := i.Use(Exports{"retrypanichost/retrypanichost": {
		"Mark": reflect.ValueOf(func() { calls.Add(1) }),
	}}); err != nil {
		t.Fatal(err)
	}

	for attempt := int32(1); attempt <= 2; attempt++ {
		_, err := i.EvalWithContext(context.Background(), `import "retrypanic"`)
		if err == nil {
			t.Fatalf("panicking source initializer attempt %d returned nil", attempt)
		}
		var panicErr Panic
		if !errors.As(err, &panicErr) {
			t.Fatalf("panicking source initializer attempt %d error = %T %v, want Panic", attempt, err, err)
		}
		if got := calls.Load(); got != attempt {
			t.Fatalf("panicking source initializer calls after attempt %d = %d", attempt, got)
		}
	}
}
