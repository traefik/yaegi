#!/bin/bash
# check.sh: runs the reflect battery (run.sh, the programs of cases/) with
# yaegi v2 in default mode and in AOT mode, and fails when the verdicts are not
# exactly the ones of known.tsv: a case that does not give the output of
# the compiled program and is not in known.tsv (a new silent difference,
# WRONG, or a new crash or refusal), a case of known.tsv that now passes or
# fails another way, and a case of known.tsv that does not exist any more.
# The list cannot go stale: each difference is printed, update known.tsv.
#
#   testdata/modes/check.sh [outdir]
#
# It uses the go on PATH. The cases are a module of their own (go.mod here,
# so that go test ./... of yaegi v2 does not see them); their libraries are at
# the versions of the go.mod of yaegi v2, so that the AOT copies of aot/ are
# used in AOT mode.
set -euo pipefail
cd "$(dirname "$0")"
root=$(cd ../.. && pwd)
out=${1:-$(mktemp -d)}
mkdir -p "$out"
(cd "$root" && go build -o "$out/yaegi" ./cmd/yaegi)
./run.sh -out "$out" v1="$out/yaegi" v1aot="$out/yaegi -aot" >/dev/null

# known.tsv and verdicts.tsv: case, mode, verdict, note. Every difference
# counts: a new failure, a known case that passes or fails another way (the
# line of known.tsv is wrong), a known case that was not run (removed or
# renamed: its line is stale). With CASES (run.sh runs some cases only),
# the cases that were not run do not count.
awk -F'\t' -v partial="${CASES:-}" '
	NR == FNR { if ($0 !~ /^#/ && NF >= 3) known[$1 "\t" $2] = $3; next }
	{
		k = $1 "\t" $2
		seen[k] = 1
		if ($3 == "PASS") {
			if (k in known) { printf "now PASS\t%s (known %s): remove it from known.tsv\n", k, known[k]; bad++ }
			next
		}
		if (!(k in known)) { printf "NEW %s\t%s\t%s\n", $3, k, $4; bad++; next }
		if (known[k] != $3) { printf "%s instead of %s\t%s\t%s: update known.tsv\n", $3, known[k], k, $4; bad++ }
	}
	END {
		if (partial == "") for (k in known) if (!(k in seen)) { printf "not run\t%s: remove it from known.tsv\n", k; bad++ }
		printf "%d differences with known.tsv\n", bad
		exit bad > 0
	}' known.tsv "$out/verdicts.tsv"
