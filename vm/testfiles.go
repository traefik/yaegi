package vm

// Options.IgnoreTestFiles: the loaders leave out the _test.go files, as
// yaegi does. go build reads the package clause of the test files and
// refuses a directory where a test file has another package name than the
// package or <package>_test. Traefik never runs the tests of a plugin, so
// with this option such a plugin loads.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// isTestFile reports whether name is a Go test file.
func isTestFile(name string) bool { return strings.HasSuffix(name, "_test.go") }

// readDirNoTests is the build.Context.ReadDir of the default loader with
// IgnoreTestFiles: os.ReadDir without the test files.
func readDirNoTests(dir string) ([]fs.FileInfo, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	infos := make([]fs.FileInfo, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() && isTestFile(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// moduleTestFiles returns the test files of the module of dir (the module
// whose go.mod is in dir or above it, as go list finds it; dir itself
// without one), for the go list loader with IgnoreTestFiles: an overlay
// (go help build, -overlay, see writeOverlay) deletes them. The walk starts
// from the real path of the module root (filepath.WalkDir does not follow a
// symbolic link, not even the root it is given), and each file is listed
// under that path and under its real path, since go list sees the module
// through the link or not. limit is Options.Root (see findModule).
func moduleTestFiles(dir, limit string) ([]string, error) {
	if root, _, _, err := findModule(dir, limit); err == nil {
		dir = root
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	var files []string
	err = filepath.WalkDir(real, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isTestFile(d.Name()) {
			return nil
		}
		files = append(files, path)
		if rel, err := filepath.Rel(real, path); err == nil && abs != real {
			files = append(files, filepath.Join(abs, rel))
		}
		return nil
	})
	return files, err
}
