#!/bin/sh
# gopatch-diff.sh: which standard library packages of the AOT copies changed
# between Go patch releases. It answers "does yaegi v2 need new copies for this
# Go patch release?" (docs/DEVELOPMENT.md, "Go patch releases").
#
#   sh scripts/gopatch-diff.sh 1.27 1.27.0 1.27.1 1.27.2 1.27.3
#
# The first argument is the release (the directory aot/go1_NN gives the list
# of copied standard packages), the others are patch releases, in order. The
# script downloads the source of each patch release from the Go module proxy
# (golang.org/toolchain, about 70 MB each, network needed) into a temporary
# directory, and compares the .go files that are not tests of each copied
# package between two patch releases that follow each other. A package is
# "changed" when one of these files differs, was added or was removed.
# It changes nothing in the repository.
set -e
cd "$(dirname "$0")/.."
[ $# -ge 3 ] || { echo "usage: $0 <release> <patch> <patch>..." >&2; exit 2; }
rel=$1
shift
dir=aot/go$(echo "$rel" | tr . _)
[ -d "$dir" ] || { echo "$dir: no AOT copies for go$rel" >&2; exit 2; }
# The copied standard packages: the directories of aot/go1_NN with Go files
# whose first path element has no dot (aot/go1_NN/all is yaegi v2 code).
pkgs=$(cd "$dir" && find . -name '*.go' | sed 's|^\./||; s|/[^/]*$||' | sort -u | grep -v '^[^/]*\.' | grep -v '^all$' | grep -v '^_')
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
proxy=${GOPROXY_URL:-https://proxy.golang.org}
for v in "$@"; do
	m=golang.org/toolchain@v0.0.1-go$v.linux-amd64
	curl -sSfL -o "$tmp/go.zip" "$proxy/golang.org/toolchain/@v/v0.0.1-go$v.linux-amd64.zip"
	mkdir -p "$tmp/go$v"
	pat=
	for p in $pkgs; do pat="$pat $m/src/$p/*.go"; done
	# shellcheck disable=SC2086
	unzip -qo "$tmp/go.zip" $pat -d "$tmp/go$v" 2>/dev/null || true
	rm "$tmp/go.zip"
done
n=$(echo "$pkgs" | wc -l | tr -d ' ')
prev=
for v in "$@"; do
	if [ -n "$prev" ]; then
		changed=
		for p in $pkgs; do
			a=$tmp/go$prev/golang.org/toolchain@v0.0.1-go$prev.linux-amd64/src/$p
			b=$tmp/go$v/golang.org/toolchain@v0.0.1-go$v.linux-amd64/src/$p
			# Only the files of the package itself: not the tests, not
			# the subdirectories (packages of their own in the list).
			if [ -d "$a" ] && [ -d "$b" ] && ! diff -q -x '*_test.go' "$a" "$b" | grep -q '\.go'; then
				continue
			fi
			changed="$changed $p"
		done
		c=$(echo $changed | wc -w | tr -d ' ')
		echo "go$prev -> go$v: $c of $n copied packages changed:$changed"
	fi
	prev=$v
done
