package interp

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/traefik/yaegi/stdlib"
)

func TestPprofWrappers_Functions(t *testing.T) {
	i := New(Options{})

	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.Eval(`
		package geoblock
		import "runtime"
		func loadDatabase() []byte {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			return buf[:n]
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	loadDB, _ := i.Eval("geoblock.loadDatabase")
	result := loadDB.Call(nil)
	stack := string(result[0].Bytes())

	// Should contain wrapperLevel when enabled
	if !strings.Contains(stack, "wrapperLevel") {
		t.Error("expected wrapperLevel in stack when EnablePprofLabels is true")
	}

	// Verify mapping exists
	mappings := i.GetWrapperMappings()
	if len(mappings) == 0 {
		t.Error("expected wrapper mappings to be populated")
	}

	// Verify our function is in the mappings
	expectedLabel := "geoblock::loadDatabase"
	foundInMappings := false
	for _, labels := range mappings {
		for _, label := range labels {
			if label == expectedLabel {
				foundInMappings = true
				break
			}
		}
	}

	if !foundInMappings {
		t.Errorf("function label %q not found in wrapper mappings", expectedLabel)
	}
}

func TestPprofWrappers_Methods(t *testing.T) {
	i := New(Options{})

	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.Eval(`
		package testpkg
		import "runtime"
		
		type Plugin struct{}
		
		func (p *Plugin) ServeHTTP() []byte {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			return buf[:n]
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Call method via wrapper function
	_, _ = i.Eval(`package testpkg; var p = &Plugin{}; func CallMethod() []byte { return p.ServeHTTP() }`)
	callFunc, _ := i.Eval("testpkg.CallMethod")
	result := callFunc.Call(nil)
	stack := string(result[0].Bytes())

	// Should contain wrapperLevel in stack
	if !strings.Contains(stack, "wrapperLevel") {
		t.Error("expected wrapperLevel in stack for method calls")
	}

	// Verify method label is in mappings with receiver type
	expectedLabel := "testpkg:*Plugin:ServeHTTP"
	mappings := i.GetWrapperMappings()
	foundInMappings := false

	for _, labels := range mappings {
		for _, label := range labels {
			if label == expectedLabel {
				foundInMappings = true
				break
			}
		}
	}

	if !foundInMappings {
		t.Errorf("method label %q not found in wrapper mappings", expectedLabel)
	}
}

func TestPprofWrappers_InHeapProfile(t *testing.T) {
	i := New(Options{})

	_, err := i.Eval(`
		package geoblock
		func loadDatabase() []byte {
			return make([]byte, 100000)
		}
		
		type Plugin struct{}
		func (p *Plugin) checkIP() []byte {
			return make([]byte, 50000)
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Call functions to generate allocations
	loadDB, _ := i.Eval("geoblock.loadDatabase")
	for j := 0; j < 1000; j++ {
		loadDB.Call(nil)
	}

	_, _ = i.Eval(`package geoblock; var p = &Plugin{}; func CallCheck() []byte { return p.checkIP() }`)
	checkIP, _ := i.Eval("geoblock.CallCheck")
	for j := 0; j < 500; j++ {
		checkIP.Call(nil)
	}

	runtime.GC()
	var buf bytes.Buffer
	pprof.WriteHeapProfile(&buf)

	if buf.Len() == 0 {
		t.Fatal("heap profile is empty")
	}

	prof, err := profile.Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Find all wrapper functions in heap profile
	wrappersFound := make(map[string]bool)

	for _, sample := range prof.Sample {
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil && strings.Contains(line.Function.Name, "wrapperLevel") {
					wrappersFound[line.Function.Name] = true
				}
			}
		}
	}

	if len(wrappersFound) == 0 {
		t.Fatal("No wrapperLevel functions found in heap profile")
	}

	t.Logf("Found %d unique wrappers in heap profile", len(wrappersFound))
	for wrapper := range wrappersFound {
		t.Logf("  - %s", wrapper)
	}

	// Verify we can map wrappers back to functions
	mappings := i.GetWrapperMappings()

	expectedFunctions := []string{
		"geoblock::loadDatabase",
		"geoblock:*Plugin:checkIP",
	}

	for _, expectedFunc := range expectedFunctions {
		found := false
		for _, labels := range mappings {
			for _, label := range labels {
				if label == expectedFunc {
					found = true
					break
				}
			}
		}

		if !found {
			t.Errorf("function %q not found in wrapper mappings", expectedFunc)
		} else {
			t.Logf("Verified mapping: %s", expectedFunc)
		}
	}
}

func TestPprofWrappers_InCPUProfile(t *testing.T) {
	i := New(Options{})

	_, err := i.Eval(`
		package testpkg
		func busyWork() int {
			sum := 0
			for i := 0; i < 1000000; i++ {
				sum += i
			}
			return sum
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Start CPU profiling
	var buf bytes.Buffer
	if err := pprof.StartCPUProfile(&buf); err != nil {
		t.Fatal(err)
	}

	// Execute function to generate CPU samples
	busyWork, _ := i.Eval("testpkg.busyWork")
	for j := 0; j < 100; j++ {
		busyWork.Call(nil)
	}

	pprof.StopCPUProfile()

	if buf.Len() == 0 {
		t.Fatal("CPU profile is empty")
	}

	// Parse CPU profile
	prof, err := profile.Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Look for wrapper in CPU profile
	foundWrapper := false

	for _, sample := range prof.Sample {
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil && strings.Contains(line.Function.Name, "wrapperLevel") {
					foundWrapper = true
					t.Logf("Found wrapper in CPU profile: %s", line.Function.Name)
					break
				}
			}
		}
	}

	if !foundWrapper {
		t.Error("No wrapperLevel functions found in CPU profile")
	}

	// Verify mapping
	mappings := i.GetWrapperMappings()
	expectedLabel := "testpkg::busyWork"
	found := false

	for _, labels := range mappings {
		for _, label := range labels {
			if label == expectedLabel {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("function %q not in wrapper mappings", expectedLabel)
	}
}

func TestPprofLabel_Format(t *testing.T) {
	i := New(Options{})

	tests := []struct {
		code          string
		expectedLabel string
	}{
		{
			code: `
				package mypkg
				func MyFunc() int { return 1 }
			`,
			expectedLabel: "mypkg::MyFunc",
		},
		{
			code: `
				package test
				type Handler struct{}
				func (h *Handler) ServeHTTP() int { return 200 }
			`,
			expectedLabel: "test:*Handler:ServeHTTP",
		},
		{
			code: `
				package plugin
				type Config struct{}
				func (c Config) Validate() bool { return true }
			`,
			expectedLabel: "plugin:Config:Validate",
		},
	}

	for _, tt := range tests {
		_, err := i.Eval(tt.code)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Verify all expected functions are in wrapper mappings
	mappings := i.GetWrapperMappings()

	for _, tt := range tests {
		found := false
		for _, labels := range mappings {
			for _, label := range labels {
				if label == tt.expectedLabel {
					found = true
					break
				}
			}
		}

		if !found {
			t.Errorf("expected label %q not found in wrapper mappings", tt.expectedLabel)
		}
	}
}

func TestGetWrapperMappings_Complete(t *testing.T) {
	i := New(Options{})

	// Create multiple functions and methods
	_, err := i.Eval(`
		package plugin1
		func Init() int { return 1 }
		func Process() int { return 2 }
		
		type Handler struct{}
		func (h *Handler) ServeHTTP() int { return 3 }
		func (h *Handler) Cleanup() int { return 4 }
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = i.Eval(`
		package plugin2
		func Start() int { return 5 }
		
		type Worker struct{}
		func (w *Worker) Run() int { return 6 }
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Get mappings
	mappings := i.GetWrapperMappings()

	// Verify we have mappings
	totalFunctions := 0
	for level, labels := range mappings {
		totalFunctions += len(labels)
		t.Logf("Wrapper level %d: %v", level, labels)
	}

	expectedCount := 6
	if totalFunctions != expectedCount {
		t.Errorf("expected %d functions in mappings, got %d", expectedCount, totalFunctions)
	}

	// Verify incremental assignment (first 6 should be levels 0-5)
	for level := 0; level < 6; level++ {
		if labels, exists := mappings[level]; !exists || len(labels) == 0 {
			t.Errorf("expected wrapper level %d to be assigned", level)
		}
	}

	// Verify all expected functions are mapped
	expectedLabels := []string{
		"plugin1::Init",
		"plugin1::Process",
		"plugin1:*Handler:ServeHTTP",
		"plugin1:*Handler:Cleanup",
		"plugin2::Start",
		"plugin2:*Worker:Run",
	}

	allFound := make(map[string]bool)
	for _, labels := range mappings {
		for _, label := range labels {
			allFound[label] = true
		}
	}

	for _, expected := range expectedLabels {
		if !allFound[expected] {
			t.Errorf("expected function %q not found in mappings", expected)
		}
	}
}
