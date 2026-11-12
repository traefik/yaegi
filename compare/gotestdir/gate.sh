#!/usr/bin/env bash
# gate.sh: runs the official Go test suite (GOROOT/test of go1.27.3, all the
# actions of gotestdir) under yaegi v2 only, and fails if a test that passes in
# results/govm.tsv (the frozen pass list) does not pass any more. New passes
# are printed but do not fail: update results/govm.tsv with run.sh to freeze
# them.
#
#   compare/gotestdir/gate.sh [jobs] [timeout]
#
# It uses the go on PATH, which must be a go1.27 release (the CI pins it);
# run.sh gets go1.27.3 itself. Needs the network once for the sparse clone of
# golang/go (test/ only); set GOTESTDIR_ROOT to use an existing copy.
set -euo pipefail
cd "$(dirname "$0")"
D=$(pwd)
ROOT=$(cd ../.. && pwd)
JOBS=${1:-4}
TIMEOUT=${2:-60s}
GOVER=go1.27.3

mkdir -p out
C=${XDG_CACHE_HOME:-$HOME/.cache}/yaegi-gotestdir/$GOVER
T=${GOTESTDIR_ROOT:-$C/test}
if [ ! -d "$T" ]; then
	git clone -q --depth 1 --branch "$GOVER" --filter=blob:none --sparse https://github.com/golang/go "$C"
	git -C "$C" sparse-checkout set test
fi
go version

(cd "$ROOT" && go build -o "$D/out/yaegi-v2" ./cmd/yaegi)
(cd "$ROOT/compare" && go build -o "$D/out/gotestdir" ./gotestdir &&
	go build -o "$D/out/loadcheck" ./gotestdir/loadcheck)
# All the actions of gotestdir, as run.sh measures results/govm.tsv: the
# compile-like tests with loadcheck, the generators of runoutput with go. The
# engine key of yaegi v2 is "govm" (its name when the pass list was frozen).
run() { (cd "$ROOT" && "$D/out/gotestdir" -root "$T" -engine govm -bin "$D/out/yaegi-v2" -load "$D/out/loadcheck" -go "$(command -v go)" -timeout "$TIMEOUT" "$@"); }
run -j "$JOBS" > out/govm.tsv

echo "== now"
cut -f2,3 out/govm.tsv | sort | uniq -c

# A frozen pass that fails is run once more, alone: a few tests depend on
# timing or on when a finalizer runs (typeparam/chans.go), and a loaded
# runner makes them fail now and then. The second result replaces the first.
again=$(awk -F'\t' '
	NR == FNR { if ($3 == "PASS") frozen[$1] = 1; next }
	($1 in frozen) && $3 != "PASS" { print $1 }' results/govm.tsv out/govm.tsv)
for f in $again; do
	echo "== again, alone: $f"
	rx="^$(printf '%s' "$f" | sed 's/[.]/\\./g')\$"
	run -j 1 -only "$rx" > out/again.tsv
	awk -F'\t' -v f="$f" 'NR == FNR { if ($1 == f) row = $0; next } $1 == f && row != "" { print row; next } { print }' out/again.tsv out/govm.tsv > out/govm.tsv.new
	mv out/govm.tsv.new out/govm.tsv
	grep -F "$f	" out/again.tsv | cut -f1,3 || true
done

# Columns: file, action, status. A test lost or not PASS is a regression.
awk -F'\t' '
	NR == FNR { if ($3 == "PASS") { frozen[$1] = 1; n++ }; next }
	{ now[$1] = $3 }
	END {
		for (f in frozen) {
			s = (f in now) ? now[f] : "missing"
			if (s != "PASS") { printf "REGRESSION\t%s\t%s\n", f, s; bad++ }
		}
		for (f in now) if (now[f] == "PASS" && !(f in frozen)) printf "new pass\t%s\n", f
		printf "%d frozen passes, %d regressions\n", n, bad
		exit (bad > 0 ? 1 : 0)
	}' results/govm.tsv out/govm.tsv
