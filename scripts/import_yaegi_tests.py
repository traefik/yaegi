#!/usr/bin/env python3
"""Import the yaegi conformance tests into tests/yaegi, with provenance.

Usage: scripts/import_yaegi_tests.py <yaegi checkout>

Read-only on the yaegi checkout. It expects:
  - the local branch `master`;
  - the fork branches listed in FORK_BRANCHES;
  - upstream pull request heads fetched as refs/remotes/upstream-pr/<N>, e.g.
      git fetch --no-tags https://github.com/traefik/yaegi +pull/<N>/head:refs/remotes/upstream-pr/<N>

Rules (see tests/yaegi/README.md):
  - master: every file of _test/ is copied unchanged (git archive), plus the
    API unit tests (interp/, cmd/yaegi/, extract/ *_test.go) into _api/master/.
  - branch / PR, relative to its merge base with master:
      * added _test/ files go to _test/ (same relative path); on a name
        collision with different content, the file is suffixed
        (.fork-<branch>.go, .upstream-pr<N>.go); identical content is skipped;
      * modified _test/ files are imported as a suffixed variant only when
        their trailing expectation comment differs from master's;
      * added/modified interp/testdata/ files go to _interp-testdata/<origin>/;
      * added *_test.go files go to _api/<origin>/; modified ones are stored
        as a patch _api/<origin>/<path>.diff;
      * added extract/testdata/ files go to _api/<origin>/extract/testdata/.
Directories starting with "_" are ignored by the go tool, so nothing imported
here is built by `go build ./...` or `go test ./...`.
  - PR 1727 (mechanical interface{} -> any rewrite): modified files ignored.
"""
import io
import os
import re
import subprocess
import sys
import tarfile

FORK_BRANCHES = [
    "ai/fix-upstream-1414",
    "ai/fix-empty-typeswitch-clauses",
    "ai/fix-go-build-constraints",
    "ai/fix-upstream-1341",
    "ai/fix-upstream-1518",
    "ai/fix-upstream-1437",
    "ai/fix-upstream-1674",
]
UPSTREAM_PRS = [1737, 1733, 1732, 1730, 1729, 1727, 1724, 1712, 1711, 1708, 1647, 1638, 1560, 1357, 1265, 1081, 1079]
MECHANICAL_PRS = {1727: "mechanical interface{} -> any rewrite: modified files not imported"}
API_DIRS = ("interp/", "cmd/yaegi/", "extract/")

repo = os.path.abspath(sys.argv[1])
here = os.path.dirname(os.path.abspath(__file__))
dest = os.path.join(here, "..", "tests", "yaegi")
manifest = []  # (path, origin, status, note)
notes = []  # human-readable notes for things not imported


def git(*args, binary=False):
    out = subprocess.check_output(["git", "-C", repo] + list(args))
    return out if binary else out.decode()


def write(rel, data):
    p = os.path.join(dest, rel)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    with open(p, "wb") as f:
        f.write(data)


def read(rel):
    p = os.path.join(dest, rel)
    if not os.path.exists(p):
        return None
    with open(p, "rb") as f:
        return f.read()


def expectation(src):
    """Trailing comment block of a Go file (the TestFile expectation)."""
    lines = src.decode(errors="replace").rstrip().splitlines()
    block = []
    for line in reversed(lines):
        if line.startswith("//"):
            block.append(line)
        else:
            break
    return "\n".join(reversed(block))


def has_expectation(src):
    exp = expectation(src)
    return re.search(r"^// (Output|Error):", exp, re.M) is not None


def suffixed(rel, suffix):
    base, ext = os.path.splitext(rel)
    return base + "." + suffix + ext


master_sha = git("rev-parse", "master").strip()
master_origin = "master@" + master_sha

# 1. master _test/, unchanged.
tar = tarfile.open(fileobj=io.BytesIO(git("archive", "master", "_test", binary=True)))
for m in tar.getmembers():
    if m.isfile():
        write(m.name, tar.extractfile(m).read())
        manifest.append((m.name, master_origin, "added", ""))
    elif m.issym():
        notes.append("master: symlink %s -> %s not imported" % (m.name, m.linkname))

# 4. API unit tests of master.
for path in git("ls-tree", "-r", "--name-only", "master").splitlines():
    if path.startswith(API_DIRS) and path.endswith("_test.go") and "/testdata/" not in path:
        rel = os.path.join("_api", "master", path)
        write(rel, git("show", "master:" + path, binary=True))
        manifest.append((rel, master_origin, "added", "API reference test (does not compile against the VM)"))


def import_ref(ref, origin_label, suffix, mechanical=None):
    sha = git("rev-parse", ref).strip()
    origin = "%s@%s" % (origin_label, sha)
    base = git("merge-base", "master", ref).strip()
    odir = suffix  # per-origin sub-directory for _api/ and _interp-testdata/
    for line in git("diff", "--name-status", "--no-renames", base, ref).splitlines():
        status, path = line.split("\t", 1)
        if status == "D" or mechanical and status == "M":
            continue
        if path.startswith("_test/"):
            data = git("show", "%s:%s" % (ref, path), binary=True)
            existing = read(path)
            if status == "A":
                if existing is None:
                    write(path, data)
                    note = ""
                    if path.endswith(".go") and path.count("/") == 1 and not has_expectation(data):
                        note = "no Output:/Error: expectation"
                    manifest.append((path, origin, "added", note))
                elif existing == data:
                    notes.append("%s: %s identical to the already imported file, skipped" % (origin_label, path))
                else:
                    rel = suffixed(path, suffix)
                    write(rel, data)
                    manifest.append((rel, origin, "variant", "added by %s with the name of an existing file of different content" % origin_label))
            elif status == "M":
                try:
                    master_data = git("show", "master:" + path, binary=True)
                except subprocess.CalledProcessError:
                    master_data = None
                if master_data == data or master_data is not None and path.endswith(".go") and expectation(master_data) == expectation(data):
                    notes.append("%s: %s modified, expectation unchanged vs master, not imported" % (origin_label, path))
                    continue
                rel = suffixed(path, suffix)
                write(rel, data)
                manifest.append((rel, origin, "variant", "expectation differs from master's %s" % os.path.basename(path)))
            continue
        if path.startswith("interp/testdata/"):
            rel = os.path.join("_interp-testdata", odir, path[len("interp/testdata/"):])
            write(rel, git("show", "%s:%s" % (ref, path), binary=True))
            manifest.append((rel, origin, "added" if status == "A" else "variant", "interp/testdata/" + path[len("interp/testdata/"):]))
            continue
        if path.startswith("extract/testdata/"):
            rel = os.path.join("_api", odir, path)
            write(rel, git("show", "%s:%s" % (ref, path), binary=True))
            manifest.append((rel, origin, "added" if status == "A" else "variant", "testdata of the extract API tests"))
            continue
        if path.endswith("_test.go"):
            if status == "A":
                rel = os.path.join("_api", odir, path)
                write(rel, git("show", "%s:%s" % (ref, path), binary=True))
                manifest.append((rel, origin, "added", "API test added by %s" % origin_label))
            else:
                rel = os.path.join("_api", odir, path + ".diff")
                write(rel, git("diff", base, ref, "--", path, binary=True))
                manifest.append((rel, origin, "variant", "patch of %s against its merge base %s" % (path, base[:12])))
    if mechanical:
        notes.append("%s: %s" % (origin_label, mechanical))


for b in FORK_BRANCHES:
    import_ref(b, "fork:" + b, "fork-" + b.split("/", 1)[1])
for n in UPSTREAM_PRS:
    import_ref("refs/remotes/upstream-pr/%d" % n, "upstream-pr:%d" % n, "upstream-pr%d" % n, MECHANICAL_PRS.get(n))

with open(os.path.join(dest, "MANIFEST.tsv"), "w") as f:
    f.write("path\torigin\tstatus\tnote\n")
    for row in sorted(manifest):
        f.write("\t".join(row) + "\n")
with open(os.path.join(dest, "IMPORT-NOTES.txt"), "w") as f:
    f.write("Files seen but not imported (generated by scripts/import_yaegi_tests.py)\n\n")
    for n in notes:
        f.write(n + "\n")
print("imported %d files, %d notes" % (len(manifest), len(notes)))
