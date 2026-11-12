package vm

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestFieldTrackOn(t *testing.T) {
	for s, want := range map[string]bool{
		"":                         false,
		"fieldtrack":               true,
		"jsonv2,fieldtrack":        true,
		"fieldtrack,nofieldtrack":  false,
		"nofieldtrack, fieldtrack": true,
		"fieldtrack,none":          false,
		"none,fieldtrack":          true,
		"nofieldtrackx":            false,
	} {
		if got := fieldTrackOn(s); got != want {
			t.Errorf("fieldTrackOn(%q) = %v, want %v", s, got, want)
		}
	}
}

// TestGoListFieldTrack checks that fieldtrack comes from the go command (as
// the build tags of go list), not only from the GOEXPERIMENT variable: here
// it is set with a go env file (`go env -w`).
func TestGoListFieldTrack(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	dir := t.TempDir()
	src := "package main\n\ntype U struct{}\n\n//go:nointerface\nfunc (*U) Bad() {}\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	goenv := filepath.Join(dir, "goenv")
	if err := os.WriteFile(goenv, []byte("GOEXPERIMENT=fieldtrack\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOEXPERIMENT", "")
	t.Setenv("GOENV", goenv)
	cfg := &packages.Config{Mode: packages.LoadSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, "./main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !goListFieldTrack(dir, pkgs) {
		t.Error("goListFieldTrack = false with GOEXPERIMENT=fieldtrack in the go env file")
	}
	t.Setenv("GOENV", "off")
	if goListFieldTrack(dir, pkgs) {
		t.Error("goListFieldTrack = true without fieldtrack")
	}
}
