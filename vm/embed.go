package vm

// //go:embed for the packages run from source.
//
// gc fills a package variable that has a //go:embed directive with the
// content of files of the package directory, before the initializers run.
// The VM does the same at load time, for the three kinds of variable of gc:
// a string, a []byte (protobuf's internal/editiondefaults embeds its edition
// defaults) and an embed.FS (OPA embeds its capability files). Then
// Machine.globalPtr gives the variable this content.
//
// The files are selected with the rules of the go command
// (cmd/go/internal/load, resolveEmbed): a pattern is a path.Match pattern
// relative to the package directory, optionally prefixed with "all:", with no
// "." or ".." element and no leading or trailing slash. A pattern that names
// a directory embeds the files of its tree, without the names that start
// with "." or "_" (unless "all:"), without the symbolic links and other
// irregular files, and without the trees of other modules (a go.mod). A
// pattern that names a symbolic link or an irregular file, a file in a
// directory of another module, or under a symbolic link, is an error, as are
// the names ".git", ".hg", ".svn", ".bzr" and the names that a module cannot
// hold. The files are read through an os.Root of the package directory:
// nothing is read outside it, even if a file is changed during the load.
//
// An embed.FS is the real type of package embed, so its methods, io/fs and
// net/http see it as in gc. Its files are in the order of gc
// (cmd/compile/internal/staticdata, embedFileLess), with an entry for each
// directory. The total size and the number of embedded files are bounded
// (embedLimits), as the rest of the load (LoadLimits).

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/mod/module"
)

const (
	// maxEmbedBytes is the total size of the embedded files of a load,
	// whatever the LoadLimits.
	maxEmbedBytes = 64 << 20
	// maxEmbedEntries is the number of files and directories that the
	// patterns of a load may visit, whatever the LoadLimits.
	maxEmbedEntries = 1 << 16
)

// embedValue is the content of a //go:embed variable.
type embedValue struct {
	data  string      // a string or []byte variable
	files []embedFile // an embed.FS variable (isFS)
	isFS  bool
}

// embedFile has the layout of the file type of package embed (name, data,
// hash), which gc fills (cmd/compile/internal/staticdata, WriteEmbed). A
// directory has a name that ends with "/" and no data.
type embedFile struct {
	name string
	data string
	hash [16]byte // a truncated SHA-256 of data; package embed does not read it
}

// embedFSLayout reports whether embed.FS is a struct with one field, a
// pointer to a slice of a struct with the fields of embedFile at the same
// offsets: then newEmbedFS can fill it. It is checked once, so that a Go
// release that changes the type refuses embed.FS variables instead of
// building a wrong value.
var embedFSLayout = sync.OnceValue(func() bool {
	ft := reflect.TypeFor[embed.FS]()
	if ft.Kind() != reflect.Struct || ft.NumField() != 1 || ft.Size() != unsafe.Sizeof(uintptr(0)) {
		return false
	}
	f := ft.Field(0)
	if f.Name != "files" || f.Offset != 0 || f.Type.Kind() != reflect.Pointer || f.Type.Elem().Kind() != reflect.Slice {
		return false
	}
	et, mine := f.Type.Elem().Elem(), reflect.TypeFor[embedFile]()
	if et.Kind() != reflect.Struct || et.NumField() != mine.NumField() || et.Size() != mine.Size() || et.Align() != mine.Align() {
		return false
	}
	for i := range et.NumField() {
		a, b := et.Field(i), mine.Field(i)
		if a.Name != b.Name || a.Offset != b.Offset || a.Type != b.Type {
			return false
		}
	}
	return true
})

// newEmbedFS returns an embed.FS of the files (sorted as gc sorts them).
func newEmbedFS(files []embedFile) embed.FS {
	var fsys embed.FS
	if !embedFSLayout() {
		panic("vm: embed.FS has an unknown layout") // refused at load time (embedsOf)
	}
	list := files
	*(**[]embedFile)(unsafe.Pointer(&fsys)) = &list
	return fsys
}

// embedLimits bounds the //go:embed files of a load.
type embedLimits struct {
	maxBytes   int64
	maxEntries int
	bytes      int64
	entries    int
}

func newEmbedLimits(l LoadLimits) *embedLimits {
	e := &embedLimits{maxBytes: maxEmbedBytes, maxEntries: maxEmbedEntries}
	if l.MaxTotalSourceBytes > 0 && l.MaxTotalSourceBytes < e.maxBytes {
		e.maxBytes = l.MaxTotalSourceBytes
	}
	if l.MaxFiles > 0 && l.MaxFiles < e.maxEntries {
		e.maxEntries = l.MaxFiles
	}
	return e
}

func (e *embedLimits) visit() error {
	if e.entries++; e.entries > e.maxEntries {
		return fmt.Errorf("more than %d embedded files and directories (LoadLimits)", e.maxEntries)
	}
	return nil
}

func (e *embedLimits) add(n int64) error {
	if e.bytes += n; e.bytes > e.maxBytes {
		return fmt.Errorf("more than %d bytes of embedded files (LoadLimits)", e.maxBytes)
	}
	return nil
}

// embedsOf returns the content of the //go:embed variables of the packages
// of src, keyed by variable.
func embedsOf(src []sourcePkg, limits LoadLimits) (map[*types.Var]embedValue, error) {
	var out map[*types.Var]embedValue
	lim := newEmbedLimits(limits)
	for _, p := range src {
		for _, f := range p.files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, s := range gd.Specs {
					vs, ok := s.(*ast.ValueSpec)
					if !ok {
						continue
					}
					pats, err := embedPatterns(gd, vs)
					if err != nil {
						return nil, err
					}
					if len(pats) == 0 {
						continue
					}
					pos := p.fset.Position(vs.Pos())
					if !importsEmbed(f) {
						return nil, fmt.Errorf("%s: go:embed only allowed in Go files that import \"embed\"", pos)
					}
					if len(vs.Names) != 1 || len(vs.Values) != 0 {
						return nil, fmt.Errorf("%s: go:embed needs a single variable without a value", pos)
					}
					obj, _ := p.types.Scope().Lookup(vs.Names[0].Name).(*types.Var)
					if obj == nil {
						return nil, fmt.Errorf("%s: go:embed only applies to package variables", pos)
					}
					isFS := isEmbedFS(obj.Type())
					if !isFS && !isString(obj.Type()) && !isByteSliceType(obj.Type()) {
						return nil, fmt.Errorf("%s: go:embed cannot apply to var of type %s", pos, obj.Type())
					}
					if isFS && !embedFSLayout() {
						return nil, fmt.Errorf("%s: go:embed into an embed.FS: the embed.FS of this Go release has an unknown layout", pos)
					}
					v, err := embedRead(filepath.Dir(pos.Filename), pats, isFS, lim)
					if err != nil {
						return nil, fmt.Errorf("%s: %v", pos, err)
					}
					if out == nil {
						out = map[*types.Var]embedValue{}
					}
					out[obj] = v
				}
			}
		}
	}
	return out, nil
}

// importsEmbed reports whether f imports package embed (gc refuses a
// //go:embed in a file that does not).
func importsEmbed(f *ast.File) bool {
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == "embed" {
			return true
		}
	}
	return false
}

// isEmbedFS reports whether t is embed.FS.
func isEmbedFS(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "embed" && n.Obj().Name() == "FS"
}

// isByteSliceType reports whether t is a []byte (or a named type of it).
func isByteSliceType(t types.Type) bool {
	s, ok := t.Underlying().(*types.Slice)
	return ok && isByteType(s.Elem())
}

// embedPatterns returns the patterns of the //go:embed lines in the doc
// comment of vs (or of its declaration when it has a single spec).
func embedPatterns(gd *ast.GenDecl, vs *ast.ValueSpec) ([]string, error) {
	docs := []*ast.CommentGroup{vs.Doc}
	if len(gd.Specs) == 1 {
		docs = append(docs, gd.Doc)
	}
	var pats []string
	for _, cg := range docs {
		if cg == nil {
			continue
		}
		for _, c := range cg.List {
			rest, ok := strings.CutPrefix(c.Text, "//go:embed")
			if !ok || rest != "" && rest[0] != ' ' && rest[0] != '\t' {
				continue
			}
			ps, err := splitEmbed(rest)
			if err != nil {
				return nil, err
			}
			pats = append(pats, ps...)
		}
	}
	return pats, nil
}

// splitEmbed splits the arguments of a //go:embed line: words, or quoted
// strings ("a b" or `a b`).
func splitEmbed(s string) ([]string, error) {
	var out []string
	s = strings.TrimSpace(s)
	for s != "" {
		var w string
		switch s[0] {
		case '"', '`':
			q, err := strconv.QuotedPrefix(s)
			if err != nil {
				return nil, fmt.Errorf("go:embed: bad quoted pattern %s", s)
			}
			w, _ = strconv.Unquote(q)
			s = s[len(q):]
		default:
			i := strings.IndexAny(s, " \t")
			if i < 0 {
				i = len(s)
			}
			w, s = s[:i], s[i:]
		}
		out = append(out, w)
		s = strings.TrimSpace(s)
	}
	return out, nil
}

// embedRead resolves the patterns in dir and reads the files: one file for
// a string or a []byte, the files and their directories for an embed.FS.
func embedRead(dir string, pats []string, isFS bool, lim *embedLimits) (embedValue, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return embedValue{}, fmt.Errorf("go:embed: %v", err)
	}
	defer root.Close()
	// The files of each pattern, in the order of the patterns, each file
	// once (embedFileList of gc).
	have := map[string]bool{}
	var list []string
	for _, pat := range pats {
		files, err := embedResolve(root, pat, lim)
		if err != nil {
			return embedValue{}, fmt.Errorf("pattern %s: %v", pat, err)
		}
		for _, f := range files {
			if !have[f] {
				have[f] = true
				list = append(list, f)
			}
			if isFS {
				for d := path.Dir(f); d != "." && !have[d+"/"]; d = path.Dir(d) {
					have[d+"/"] = true
					list = append(list, d+"/")
				}
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return embedFileLess(list[i], list[j]) })
	if !isFS {
		if len(list) != 1 {
			return embedValue{}, fmt.Errorf("invalid go:embed: multiple files for a string or []byte variable (%d)", len(list))
		}
		data, err := embedReadFile(root, list[0], lim)
		return embedValue{data: data}, err
	}
	files := make([]embedFile, len(list))
	for i, name := range list {
		files[i].name = name
		if strings.HasSuffix(name, "/") {
			continue // a directory: no data
		}
		if files[i].data, err = embedReadFile(root, name, lim); err != nil {
			return embedValue{}, err
		}
		sum := sha256.Sum256([]byte(files[i].data))
		copy(files[i].hash[:], sum[:])
	}
	return embedValue{files: files, isFS: true}, nil
}

// embedResolve returns the files (slash paths relative to the root) that
// the pattern embeds, sorted, with the rules of the go command (resolveEmbed
// in cmd/go/internal/load).
func embedResolve(root *os.Root, pattern string, lim *embedLimits) ([]string, error) {
	glob, all := strings.CutPrefix(pattern, "all:")
	if _, err := path.Match(glob, ""); err != nil || glob == "." || !fs.ValidPath(glob) {
		return nil, errors.New("invalid pattern syntax")
	}
	var matches []string
	if !strings.ContainsAny(glob, `*?[\`) {
		// As filepath.Glob (the go command), a name without a
		// metacharacter is looked up with Lstat: a symbolic link is found
		// (and refused below), not followed.
		if _, err := root.Lstat(glob); err == nil {
			matches = []string{glob}
		}
	} else {
		var err error
		if matches, err = fs.Glob(root.FS(), glob); err != nil {
			return nil, errors.New("invalid pattern syntax")
		}
	}
	dirOK := map[string]bool{}
	var list []string
	have := map[string]bool{}
	for _, rel := range matches {
		if err := lim.visit(); err != nil {
			return nil, err
		}
		info, err := root.Lstat(rel)
		if err != nil {
			return nil, err
		}
		what := "file"
		if info.IsDir() {
			what = "directory"
		}
		// The directories from the match up to the package directory: not
		// another module, real directories, valid names.
		for d := rel; d != "." && !dirOK[d]; d = path.Dir(d) {
			if _, err := root.Stat(path.Join(d, "go.mod")); err == nil {
				return nil, fmt.Errorf("cannot embed %s %s: in different module", what, rel)
			}
			if d != rel {
				if di, err := root.Lstat(d); err != nil || !di.IsDir() {
					return nil, fmt.Errorf("cannot embed %s %s: in non-directory %s", what, rel, d)
				}
			}
			dirOK[d] = true
			if elem := path.Base(d); isBadEmbedName(elem) {
				if d == rel {
					return nil, fmt.Errorf("cannot embed %s %s: invalid name %s", what, rel, elem)
				}
				return nil, fmt.Errorf("cannot embed %s %s: in invalid directory %s", what, rel, elem)
			}
		}
		switch {
		default:
			return nil, fmt.Errorf("cannot embed irregular file %s", rel)
		case info.Mode().IsRegular():
			if !have[rel] {
				have[rel] = true
				list = append(list, rel)
			}
		case info.IsDir():
			count := 0
			err := fs.WalkDir(root.FS(), rel, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err := lim.visit(); err != nil {
					return err
				}
				name := d.Name()
				if p != rel && (isBadEmbedName(name) || (name[0] == '.' || name[0] == '_') && !all) {
					// A hidden name, or a directory that a module cannot
					// hold: left out. A file with such a name is an error,
					// as with the go command (upstream-issue: 54003).
					if d.IsDir() {
						return fs.SkipDir
					}
					if name[0] == '.' || name[0] == '_' {
						return nil
					}
					return fmt.Errorf("cannot embed file %s: invalid name %s", p, name)
				}
				if d.IsDir() {
					if _, err := root.Stat(path.Join(p, "go.mod")); err == nil {
						return fs.SkipDir // another module
					}
					return nil
				}
				if !d.Type().IsRegular() {
					return nil // symbolic links and other irregular files
				}
				count++
				if !have[p] {
					have[p] = true
					list = append(list, p)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			if count == 0 {
				return nil, fmt.Errorf("cannot embed directory %s: contains no embeddable files", rel)
			}
		}
	}
	if len(list) == 0 {
		return nil, errors.New("no matching files found")
	}
	sort.Strings(list)
	return list, nil
}

// isBadEmbedName reports whether name cannot be embedded: a name that a
// module cannot hold, or the directory of a version control system.
func isBadEmbedName(name string) bool {
	if err := module.CheckFilePath(name); err != nil {
		return true
	}
	switch name {
	case "", ".bzr", ".hg", ".git", ".svn":
		return true
	}
	return false
}

// embedReadFile reads the regular file name of root, within the limits.
func embedReadFile(root *os.Root, name string, lim *embedLimits) (string, error) {
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("cannot embed irregular file %s", name)
	}
	if err := lim.add(st.Size()); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, st.Size()+1))
	if err != nil {
		return "", err
	}
	if int64(len(b)) != st.Size() {
		return "", fmt.Errorf("embed %s: the file changed during the load", name)
	}
	return string(b), nil
}

// embedFileLess is the order of the files of an embed.FS: by directory,
// then by name (embedFileLess of cmd/compile/internal/staticdata).
func embedFileLess(x, y string) bool {
	xdir, xelem := embedSplit(x)
	ydir, yelem := embedSplit(y)
	return xdir < ydir || xdir == ydir && xelem < yelem
}

func embedSplit(name string) (dir, elem string) {
	name = strings.TrimSuffix(name, "/")
	i := strings.LastIndexByte(name, '/')
	if i < 0 {
		return ".", name
	}
	return name[:i], name[i+1:]
}
