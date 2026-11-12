#!/usr/bin/env bash
# run.sh: runs the official Go test suite (GOROOT/test of go1.27.3) under
# go (reference), yaegi v2 (VM v1, VM v1 -aot, and VM v2 when YAEGI_V2 is
# set) and yaegi v0.16.1 (run and errorcheck only; GOTESTDIR_NO_YAEGI=1
# leaves it out), and writes results/*.tsv.
#
#   compare/gotestdir/run.sh [jobs] [timeout]
#
# Needs the network once: a sparse clone of golang/go (test/ only) and the
# go1.27.3 toolchain. Set GOTESTDIR_ROOT to an existing copy of test/ of
# go1.27.3 to skip the clone. The binaries must be built with go1.27: the AOT
# copies of yaegi v2 refuse another Go release. The engine key of yaegi v2 in
# the results is "govm" (its name when they were measured).
set -euo pipefail
cd "$(dirname "$0")"
D=$(pwd)
ROOT=$(cd ../.. && pwd)
JOBS=${1:-8}
TIMEOUT=${2:-60s}
GOVER=go1.27.3

mkdir -p out results
# The tests must be outside any module, as GOROOT/test is for testdir: inside
# this checkout, the go.mod of yaegi v2 would set the language version and GODEBUG.
C=${XDG_CACHE_HOME:-$HOME/.cache}/yaegi-gotestdir/$GOVER
T=${GOTESTDIR_ROOT:-$C/test}
if [ ! -d "$T" ]; then
	git clone -q --depth 1 --branch "$GOVER" --filter=blob:none --sparse https://github.com/golang/go "$C"
	git -C "$C" sparse-checkout set test
fi

export GOTOOLCHAIN=local GOFLAGS=-mod=mod GOSUMDB=off
if ! G=$(GOTOOLCHAIN=$GOVER GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org go env GOROOT 2>/dev/null); then
	echo "cannot get the $GOVER toolchain" >&2
	exit 1
fi
export PATH=$G/bin:$PATH
go version

(cd "$ROOT" && go build -o "$D/out/yaegi-v2" ./cmd/yaegi)
# The module of compare/ requires yaegi v0.
(cd "$ROOT/compare" && go build -o "$D/out/yaegi" github.com/traefik/yaegi/cmd/yaegi &&
	go build -o "$D/out/gotestdir" ./gotestdir &&
	go build -o "$D/out/loadcheck" ./gotestdir/loadcheck)

# yaegi with every symbol table (unsafe, syscall, os/exec, real os.Exit):
# the fairest setting for a conformance suite.
printf '#!/bin/sh\nshift\nexec %s run -unsafe -syscall -unrestricted "$@"\n' "$D/out/yaegi" > out/yaegi-full
chmod +x out/yaegi-full
# yaegi v2 with the AOT copies of the standard library.
printf '#!/bin/sh\nexec %s -aot "$@"\n' "$D/out/yaegi-v2" > out/yaegi-v2-aot
chmod +x out/yaegi-v2-aot

# gotestdir removes the local paths (test directory, GOROOT, GOMODCACHE,
# repository, $HOME, /tmp) from the details itself, before it cuts them.
# It runs from the repository, to find it with "go env GOMOD".
run() { (cd "$ROOT" && "$D/out/gotestdir" -root "$T" -go "$G/bin/go" -j "$JOBS" -timeout "$TIMEOUT" "$@"); }
run -engine go -bin "$G/bin/go" > results/go.tsv
# The compile-like tests (compile, compiledir, build, builddir) load the
# packages with the front end of yaegi v2 (loadcheck): the yaegi command has
# no "load only" mode. They are measured once, in govm.tsv.
run -engine govm -bin "$D/out/yaegi-v2" -load "$D/out/loadcheck" > results/govm.tsv
run -engine govm -bin "$D/out/yaegi-v2-aot" > results/govm-aot.tsv
engines="go govm govm-aot"
# v2 (experimental/vm2) is on another branch: set YAEGI_V2 to a yaegi v2
# binary built from it (go build of its command on the branch) to measure it
# too.
if [ -n "${YAEGI_V2:-}" ]; then
	printf '#!/bin/sh\nexec %s -engine=v2 "$@"\n' "$YAEGI_V2" > out/yaegi-v2-vm2
	chmod +x out/yaegi-v2-vm2
	run -engine govm -bin "$D/out/yaegi-v2-vm2" > results/govm-v2.tsv
	engines="$engines govm-v2"
fi
# yaegi: only run and errorcheck, the actions of the first measure.
if [ -z "${GOTESTDIR_NO_YAEGI:-}" ]; then
	run -engine yaegi -bin "$D/out/yaegi" -actions run,errorcheck > results/yaegi.tsv
	run -engine yaegi -bin "$D/out/yaegi-full" -actions run,errorcheck > results/yaegi-full.tsv
	engines="$engines yaegi yaegi-full"
fi

if grep -l -e /home/ -e /tmp/ results/*.tsv; then
	echo "local paths are left in the results" >&2
	exit 1
fi

for e in $engines; do
	echo "== $e"
	cut -f2,3 "results/$e.tsv" | sort | uniq -c
done
