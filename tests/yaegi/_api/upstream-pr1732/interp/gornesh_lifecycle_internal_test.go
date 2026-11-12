package interp

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

type ownerPublicationFS struct {
	fs.FS
	statChecked    chan struct{}
	readDirEntered chan struct{}
	readDirRelease chan struct{}
	statOnce       sync.Once
	readDirOnce    sync.Once
}

func (f *ownerPublicationFS) Stat(name string) (fs.FileInfo, error) {
	info, err := fs.Stat(f.FS, name)
	if name == "app" {
		f.statOnce.Do(func() { close(f.statChecked) })
	}
	return info, err
}

func (f *ownerPublicationFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "gopath/src/app" {
		f.readDirOnce.Do(func() {
			close(f.readDirEntered)
			<-f.readDirRelease
		})
	}
	return fs.ReadDir(f.FS, name)
}

func TestGorneshSourceDirectoryOwnerPublicationWaitsForPreparation(t *testing.T) {
	filesystem := &ownerPublicationFS{
		FS: fstest.MapFS{
			"gopath/src/app/main.go": {Data: []byte(`package main`)},
		},
		statChecked:    make(chan struct{}),
		readDirEntered: make(chan struct{}),
		readDirRelease: make(chan struct{}),
	}
	i := New(Options{GoPath: "gopath", SourcecodeFilesystem: filesystem})
	if _, err := i.Eval(`0`); err != nil {
		t.Fatal(err)
	}
	i.mutex.RLock()
	initialOwner := i.done
	i.mutex.RUnlock()

	i.compileMu.Lock()
	compileLocked := true
	defer func() {
		if compileLocked {
			i.compileMu.Unlock()
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalPathWithContext(ctx, "app")
		result <- err
	}()
	select {
	case <-filesystem.statChecked:
	case <-time.After(time.Second):
		t.Fatal("source-directory Eval did not inspect its path")
	}
	// Before the fix, evalPathWithCancel published its owner immediately after
	// this Stat call and only then waited for compileMu.
	time.Sleep(20 * time.Millisecond)
	i.mutex.RLock()
	ownerWhileWaiting := i.done
	i.mutex.RUnlock()
	if ownerWhileWaiting != initialOwner {
		t.Fatal("source-directory Eval published its owner before serialized preparation")
	}

	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting source-directory Eval error = %v, want context canceled", err)
	}
	i.compileMu.Unlock()
	compileLocked = false
	select {
	case <-filesystem.readDirEntered:
	case <-time.After(time.Second):
		t.Fatal("source-directory worker did not resume after preparation lock release")
	}
	close(filesystem.readDirRelease)
	// Serialize behind the abandoned worker so it cannot outlive this test.
	if _, err := i.Eval(`1`); err != nil {
		t.Fatalf("Eval after canceled source-directory preparation: %v", err)
	}
}

// TestGorneshHostSharedEstimateTracksExactly pins the invariant the O(1)
// host-shared scan fast path depends on: the estimate must return to zero
// once every host-shared object is released, and must never be zero while a
// host-shared object is still registered.
func TestGorneshHostSharedEstimateTracksExactly(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"gorneshfix/gorneshfix": {
		"Sink": reflect.ValueOf(func(v any) any { return v }),
	}}); err != nil {
		t.Fatal(err)
	}
	i.ImportUsed()
	if _, err := i.Eval(`
type payload struct{ N []int }
p := payload{N: make([]int, 8)}
gorneshfix.Sink(p)
gorneshfix.Sink(&p)
`); err != nil {
		t.Fatal(err)
	}
	i.funcMu.Lock()
	estimate := i.hostSharedEstimate
	registered := 0
	for _, obj := range i.ownedObjects {
		if obj.hostShared {
			registered++
		}
	}
	i.funcMu.Unlock()
	if estimate == 0 {
		t.Fatal("hostSharedEstimate = 0 after host-boundary marking, want > 0")
	}
	if estimate != registered {
		t.Fatalf("hostSharedEstimate = %d, registered hostShared objects = %d", estimate, registered)
	}
	// The estimate must stay exactly equal to the number of registered
	// hostShared objects across later evaluations.
	if _, err := i.Eval(`gorneshfix.Sink(p)`); err != nil {
		t.Fatal(err)
	}
	i.funcMu.Lock()
	defer i.funcMu.Unlock()
	if i.hostSharedEstimate == 0 {
		t.Fatal("hostSharedEstimate = 0 after later evaluations, want > 0")
	}
	shared := 0
	for _, obj := range i.ownedObjects {
		if obj.hostShared {
			shared++
		}
	}
	if i.hostSharedEstimate != shared {
		t.Fatalf("hostSharedEstimate = %d, registered hostShared objects = %d", i.hostSharedEstimate, shared)
	}
}
