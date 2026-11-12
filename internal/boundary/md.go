package boundary

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReadTSV reads the rows of a .tsv file written by writeTSV.
func ReadTSV(path string) ([]Row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []Row
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			return nil, fmt.Errorf("%s: bad line %q", path, line)
		}
		rows = append(rows, Row{Pkg: f[0], Entry: f[1], Kind: f[2], Check: f[3], Detail: f[4], Site: f[5]})
	}
	return rows, nil
}

// ProbeRow is one the "// row:" comments of the test programs, by probe:
// "<package> <entry>[,<entry>...] <kind> <check>", where check may be "*"
// (every check of these entries). They name the probe("id", ...) call that
// follows them.
type ProbeRow struct {
	Pkg     string
	Entries []string
	Kind    string
	Check   string
	Probe   string // file:id
}

// Covers reports whether the probe covers the row r.
func (p ProbeRow) Covers(r Row) bool {
	if p.Pkg != r.Pkg || (p.Kind != "*" && p.Kind != r.Kind) || (p.Check != "*" && p.Check != r.Check) {
		return false
	}
	for _, e := range p.Entries {
		if e == r.Entry {
			return true
		}
	}
	return false
}

// ReadProbes reads the "// row:" comments of the programs of dir.
func ReadProbes(dir string) ([]ProbeRow, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var out []ProbeRow
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		var pending []ProbeRow
		sc := bufio.NewScanner(f)
		n := 0
		for sc.Scan() {
			n++
			line := strings.TrimSpace(sc.Text())
			if rest, ok := strings.CutPrefix(line, "// row: "); ok {
				fs := strings.Fields(rest)
				if len(fs) < 4 {
					f.Close()
					return nil, fmt.Errorf("%s:%d: a row comment needs a package, entries, a kind and a check", file, n)
				}
				pending = append(pending, ProbeRow{Pkg: fs[0], Entries: strings.Split(fs[1], ","), Kind: fs[2], Check: strings.Join(fs[3:], " ")})
				continue
			}
			if i := strings.Index(line, `probe("`); i >= 0 && len(pending) > 0 {
				id := line[i+len(`probe("`):]
				id = id[:strings.Index(id, `"`)]
				for _, p := range pending {
					p.Probe = filepath.Base(file) + ":" + id
					out = append(out, p)
				}
				pending = nil
			}
		}
		f.Close()
		if len(pending) > 0 {
			return nil, fmt.Errorf("%s: row comments at the end with no probe after them", file)
		}
	}
	return out, nil
}

// KnownLine is a line of testdata/boundary/known.tsv.
type KnownLine struct {
	Probe, Mode, Class, Issue string
}

// ReadKnown reads known.tsv, by probe.
func ReadKnown(path string) (map[string][]KnownLine, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string][]KnownLine{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			return nil, fmt.Errorf("%s: bad line %q", path, line)
		}
		out[f[0]] = append(out[f[0]], KnownLine{f[0], f[1], f[2], f[3]})
	}
	return out, nil
}

// behavior says what Go does differently because of a check.
func behavior(r Row) string {
	switch r.Kind {
	case "reflect":
		switch r.Check {
		case "type name":
			return "prints or compares the name of the dynamic type"
		case "kind":
			return "takes another path for each kind of the dynamic type"
		case "length":
			return "reads the length or capacity of the value"
		case "method set":
			return "uses a method of the dynamic type found by reflect (Implements, Method)"
		case "comparability":
			return "compares the values, or refuses an uncomparable type"
		}
		return "walks the value (fields, elements, map entries) with its dynamic type"
	case "panic":
		switch {
		case strings.HasPrefix(r.Check, "map key"):
			return "the panic of an unhashable key names the dynamic type"
		case strings.HasPrefix(r.Check, "=="):
			return "the panic of an uncomparable type names the dynamic type"
		case strings.Contains(r.Check, "without ok"):
			return "the panic is a *runtime.TypeAssertionError that names the dynamic type"
		}
		return "the panic message names the dynamic type"
	}
	if r.Check == "x.(T), T unnamed" {
		return "takes a fast path for these types only: a named type of the program must not take it"
	}
	iface := strings.TrimSuffix(strings.TrimPrefix(r.Check, "x.("), ")")
	if d, ok := ifaceBehavior[iface]; ok {
		return d
	}
	return "uses the methods of " + iface + " when the dynamic type has them"
}

var ifaceBehavior = map[string]string{
	"error":                                 "uses Error() instead of the value",
	"fmt.Stringer":                          "prints String() instead of the value",
	"fmt.GoStringer":                        "prints GoString() for %#v",
	"fmt.Formatter":                         "calls Format for every verb",
	"fmt.Scanner":                           "calls Scan to read the value",
	"io.WriterTo":                           "copies with WriteTo instead of a Read loop",
	"io.ReaderFrom":                         "copies with ReadFrom instead of a Write loop",
	"io.StringWriter":                       "writes with WriteString instead of Write",
	"io.ByteReader":                         "reads one byte with ReadByte instead of Read",
	"io.ByteWriter":                         "writes one byte with WriteByte instead of Write",
	"io.RuneScanner":                        "reads with ReadRune and UnreadRune instead of a buffer of its own",
	"io.Seeker":                             "seeks instead of reading",
	"io.Closer":                             "closes the value",
	"interface{Unwrap() error}":             "follows the chain of wrapped errors",
	"interface{Unwrap() []error}":           "follows every error of a joined error",
	"interface{Is(error) bool}":             "asks the error whether it matches",
	"interface{As(any) bool}":               "asks the error to fill the target",
	"encoding/json.Marshaler":               "writes the result of MarshalJSON",
	"encoding.TextMarshaler":                "writes the result of MarshalText",
	"log/slog.LogValuer":                    "logs the result of LogValue",
	"flag.Value":                            "keeps the flag.Value of the program",
	"flag.boolFlag":                         "a flag with IsBoolFlag() true takes no argument",
	"net/http.Flusher":                      "flushes the response",
	"net/http.Hijacker":                     "takes over the connection",
	"net/http.Handler":                      "serves with the Handler of the program",
	"net.Error":                             "retries or reports a timeout with Timeout()",
	"math/rand.Source64":                    "draws 64-bit numbers with Uint64",
	"crypto.Signer":                         "signs with the key of the program",
	"crypto.Decrypter":                      "decrypts with the key of the program",
	"database/sql/driver.Valuer":            "stores the result of Value",
	"database/sql.Scanner":                  "reads a column with Scan",
	"database/sql/driver.Validator":         "asks the driver value whether it is still valid",
	"database/sql/driver.NamedValueChecker": "lets the driver check the arguments",
	"database/sql/driver.ColumnConverter":   "lets the driver convert the arguments",
	"database/sql/driver.SessionResetter":   "resets the session before reuse",
	"context.afterFuncer":                   "calls AfterFunc of the parent context",
	"expvar.Var":                            "publishes the String() of the variable",
	"io/fs.ReadDirFile":                     "lists the directory with ReadDir",
	"io/fs.StatFS":                          "uses Stat of the file system",
	"io/fs.ReadDirFS":                       "uses ReadDir of the file system",
	"io/fs.ReadFileFS":                      "uses ReadFile of the file system",
	"io/fs.GlobFS":                          "uses Glob of the file system",
	"io/fs.SubFS":                           "uses Sub of the file system",
	"io/fs.ReadLinkFS":                      "uses ReadLink of the file system",
}

// WriteMD writes dir/BOUNDARY.md (MD).
func WriteMD(dir string) error {
	b, err := MD(dir)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "BOUNDARY.md"), b, 0o644)
}

// MD returns BOUNDARY.md for the docs directory dir: the .tsv files of
// dir/boundary, the row comments of testdata/boundary (next to dir) and its
// known.tsv.
func MD(dir string) ([]byte, error) {
	tsvs, err := filepath.Glob(filepath.Join(dir, "boundary", "go1_*.tsv"))
	if err != nil {
		return nil, err
	}
	sort.Strings(tsvs)
	type merged struct {
		Row
		releases []string
	}
	var keys []string
	all := map[string]*merged{}
	var releases []string
	count := map[string]int{}
	for _, f := range tsvs {
		rel := strings.Replace(strings.TrimSuffix(filepath.Base(f), ".tsv"), "_", ".", 1)
		releases = append(releases, rel)
		rows, err := ReadTSV(f)
		if err != nil {
			return nil, err
		}
		count[rel] = len(rows)
		for _, r := range rows {
			m := all[r.Key()]
			if m == nil {
				m = &merged{Row: r}
				all[r.Key()] = m
				keys = append(keys, r.Key())
			}
			m.releases = append(m.releases, rel)
		}
	}
	sort.Strings(keys)
	root := filepath.Dir(filepath.Clean(dir))
	probes, err := ReadProbes(filepath.Join(root, "testdata", "boundary"))
	if err != nil {
		return nil, err
	}
	known, err := ReadKnown(filepath.Join(root, "testdata", "boundary", "known.tsv"))
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("# The native boundary of the standard library\n\n")
	b.WriteString("<!-- Generated by go run ./cmd/boundaryscan -md. DO NOT EDIT. -->\n\n")
	b.WriteString(`This is the inventory of every place where an exported function of the
standard library changes what it does because of the dynamic type, or the
methods, of a value that comes from its caller. A plugin of yaegi v2 gives
values of its own types to the native standard library, and native code
must see them as compiled Go would: the methods (String, WriteTo,
MarshalJSON, IsBoolFlag...), the name of the type, its kind, whether it is
comparable.

The inventory is made from the sources of GOROOT by ` + "`cmd/boundaryscan`" + `
(see its package comment for the method and its limits), once per Go
release of the support window (` + "`docs/boundary/go1_NN.tsv`" + `). It is
conservative: a check is listed when a value of the caller may reach it,
even if no plugin does it in practice.

Kinds of checks:

- **assert**: a type assertion or a type switch on a value of the caller.
  ` + "`x.(T), T unnamed`" + ` groups the assertions to unnamed types (string,
  []byte, int...): a named type of the program must not take these paths.
- **reflect**: a call of reflect on a value of the caller, by what it reads
  (the API calls are in the detail column of the .tsv files).
- **panic**: a panic whose value or message depends on the type.

Each row has a test: a probe of a program of ` + "`testdata/boundary/`" + `, run
with ` + "`go run`" + ` and with yaegi v2 in VM mode, in AOT mode and in restricted AOT mode
(` + "`-restricted -aot`" + `, as Traefik loads its plugins) by
` + "`TestBoundary`" + ` (cmd/runtests). The probes that do not give the output of Go
yet are in ` + "`testdata/boundary/known.tsv`" + `, with their issue.

`)
	fmt.Fprintf(&b, "| Release | Rows |\n|---|---|\n")
	for _, rel := range releases {
		fmt.Fprintf(&b, "| %s | %d |\n", rel, count[rel])
	}
	fmt.Fprintf(&b, "| all releases (merged) | %d |\n\n", len(keys))

	byPkg := map[string][]*merged{}
	var pkgs []string
	for _, k := range keys {
		m := all[k]
		if byPkg[m.Pkg] == nil {
			pkgs = append(pkgs, m.Pkg)
		}
		byPkg[m.Pkg] = append(byPkg[m.Pkg], m)
	}
	sort.Strings(pkgs)
	b.WriteString("Packages: ")
	for i, p := range pkgs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "[%s](#%s) (%d)", p, anchor(p), len(byPkg[p]))
	}
	b.WriteString("\n")
	for _, p := range pkgs {
		fmt.Fprintf(&b, "\n## %s\n\n", p)
		b.WriteString("| Function | Kind | Check | Go behavior | Releases | Test |\n|---|---|---|---|---|---|\n")
		for _, m := range byPkg[p] {
			var tests []string
			for _, pr := range probes {
				if pr.Covers(m.Row) {
					t := "`" + pr.Probe + "`"
					for _, k := range known[pr.Probe] {
						t += fmt.Sprintf(" (%s: %s, %s)", k.Mode, k.Class, k.Issue)
					}
					tests = append(tests, t)
				}
			}
			test := "**none**"
			if len(tests) > 0 {
				test = strings.Join(tests, "<br>")
			}
			rels := strings.Join(m.releases, " ")
			if len(m.releases) == len(releases) {
				rels = "all"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s |\n", m.Entry, m.Kind, mdCode(m.Check), behavior(m.Row), rels, test)
		}
	}
	return []byte(b.String()), nil
}

func anchor(s string) string {
	return strings.NewReplacer("/", "", ".", "").Replace(strings.ToLower(s))
}

// mdCode puts the Go part of a check in code.
func mdCode(c string) string {
	if strings.HasPrefix(c, "x.(") {
		// the type may contain parentheses: the code part ends before
		// " without" or ", T unnamed"
		end := len(c)
		if j := strings.Index(c, " without"); j >= 0 {
			end = j
		} else if j := strings.Index(c, ", T unnamed"); j >= 0 {
			end = j
		}
		return "`" + c[:end] + "`" + c[end:]
	}
	return c
}

// Uncovered prints the rows of the packages pkgs (comma-separated, or
// "all") of the .tsv files of the docs directory dir that have no probe,
// and the row comments of these packages that match no row. It returns
// the number of lines printed.
func Uncovered(dir, pkgs string, w io.Writer) (int, error) {
	want := map[string]bool{}
	for _, p := range strings.Split(pkgs, ",") {
		want[p] = true
	}
	in := func(p string) bool { return want["all"] || want[p] }
	tsvs, err := filepath.Glob(filepath.Join(dir, "boundary", "go1_*.tsv"))
	if err != nil {
		return 0, err
	}
	probes, err := ReadProbes(filepath.Join(filepath.Dir(filepath.Clean(dir)), "testdata", "boundary"))
	if err != nil {
		return 0, err
	}
	used := make([]bool, len(probes))
	n := 0
	seen := map[string]bool{}
	for _, f := range tsvs {
		rows, err := ReadTSV(f)
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			ok := false
			for i, p := range probes {
				if p.Covers(r) {
					ok, used[i] = true, true
				}
			}
			if !ok && in(r.Pkg) && !seen[r.Key()] {
				seen[r.Key()] = true
				fmt.Fprintf(w, "no probe: %s %s %s %s (%s)\n", r.Pkg, r.Entry, r.Kind, r.Check, filepath.Base(f))
				n++
			}
		}
	}
	for i, p := range probes {
		if !used[i] && in(p.Pkg) {
			fmt.Fprintf(w, "matches no row: %s: // row: %s %s %s %s\n", p.Probe, p.Pkg, strings.Join(p.Entries, ","), p.Kind, p.Check)
			n++
		}
	}
	return n, nil
}
