package vm

import (
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Load errors name the files of the plugin (a type error is
// "<file>:12:3: undefined: x"), and so the directories of the host: where
// the host keeps its plugins, the module cache, GOROOT. A host such as
// Traefik shows these errors in its logs and its dashboard. hidePaths gives
// the message with these directories replaced by short names:
//
//   - Options.Dir, the plugin: "." (as go build prints the files of the
//     current directory);
//   - Options.Root: "$ROOT"; Options.GoPath: "$GOPATH";
//   - the module cache and GOROOT: "$GOMODCACHE" and "$GOROOT".
//
// The error itself stays in the chain: errors.Is and errors.As see it (a
// *LoadError, a *PreflightError...), and a host that wants the full paths
// can read them there (errors.Unwrap). Only the text of Error changes.

// pathsHidden is a load error whose message names no host directory.
type pathsHidden struct {
	msg string
	err error
}

func (e *pathsHidden) Error() string { return e.msg }
func (e *pathsHidden) Unwrap() error { return e.err }

// hidePaths returns err with the host directories of opts replaced in its
// message (see the top of the file).
func (opts *Options) hidePaths(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	out := hostPathNames(opts).replace(msg)
	if out == msg {
		return err
	}
	return &pathsHidden{out, err}
}

// pathNames replaces host directories by short names in a message.
type pathNames []pathName

type pathName struct {
	path, name string
}

// hostPathNames returns the host directories of a load with opts, the
// longest first, so that the most precise name wins.
func hostPathNames(opts *Options) pathNames {
	type dir struct{ path, name string }
	var dirs []dir
	add := func(path, name string) {
		if path == "" {
			return
		}
		forms := []string{path}
		if abs, err := filepath.Abs(path); err == nil {
			forms = append(forms, abs)
			if real, err := filepath.EvalSymlinks(abs); err == nil {
				forms = append(forms, real)
			}
		}
		for _, f := range forms {
			f = filepath.Clean(f)
			if len(f) > 1 && f != filepath.VolumeName(f)+string(filepath.Separator) {
				dirs = append(dirs, dir{f, name})
			}
		}
	}
	add(opts.Dir, ".")
	add(opts.Root, "$ROOT")
	add(opts.GoPath, "$GOPATH")
	add(modCache(), "$GOMODCACHE")
	add(build.Default.GOROOT, "$GOROOT")
	slices.SortStableFunc(dirs, func(a, b dir) int { return len(b.path) - len(a.path) })
	var ns pathNames
	for _, d := range dirs {
		ns = append(ns, pathName(d))
	}
	return ns
}

func (ns pathNames) replace(msg string) string {
	for _, n := range ns {
		msg = replaceDir(msg, n.path, n.name)
	}
	return msg
}

// replaceDir replaces dir by name in msg where it is a whole path or the
// start of one (followed by a separator): not a longer name that starts or
// ends the same (/plugins and /plugins2, /go and /usr/go).
func replaceDir(msg, dir, name string) string {
	var b strings.Builder
	for {
		i := strings.Index(msg, dir)
		if i < 0 {
			break
		}
		end := i + len(dir)
		if (i == 0 || !nameByte(msg[i-1])) && (end == len(msg) || !nameByte(msg[end])) {
			b.WriteString(msg[:i])
			b.WriteString(name)
		} else {
			b.WriteString(msg[:end])
		}
		msg = msg[end:]
	}
	b.WriteString(msg)
	return b.String()
}

// nameByte reports whether c can be part of a file name next to a
// directory: then the directory is part of a longer name.
func nameByte(c byte) bool {
	return c == '_' || c == '.' || c == '-' || c == '~' || c >= 0x80 ||
		'0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// modCache returns the module cache directory, as the go command finds it.
func modCache() string {
	if d := os.Getenv("GOMODCACHE"); d != "" {
		return d
	}
	if gp := filepath.SplitList(build.Default.GOPATH); len(gp) > 0 && gp[0] != "" {
		return filepath.Join(gp[0], "pkg", "mod")
	}
	return ""
}
