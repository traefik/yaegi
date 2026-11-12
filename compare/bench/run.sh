#!/usr/bin/env bash
# Reproduces the W1/W2/W3 measurements of docs/BENCHMARKS.md.
#
#   compare/bench/run.sh [label] [count] [benchtime]
#
# label: output sub-directory under compare/bench/results/ (default: "run")
# Network-free build: uses the local module cache only.
set -euo pipefail
cd "$(dirname "$0")"
D=$(pwd)

LABEL=${1:-run}
COUNT=${2:-10}
BENCHTIME=${3:-300ms}
OUT=$D/results/$LABEL
mkdir -p "$OUT"

export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off

go version > "$OUT/env.txt"
uname -a >> "$OUT/env.txt"
(system_profiler SPHardwareDataType 2>/dev/null | grep -E "Model Name|Chip|Cores|Memory") >> "$OUT/env.txt" || true
uptime >> "$OUT/env.txt"
(git rev-parse --short HEAD 2>/dev/null || echo "no git") >> "$OUT/env.txt"
go list -m github.com/traefik/yaegi >> "$OUT/env.txt"

go build -o bin/bench .
# bench runs from the repository root: it loads plugins/ from its working
# directory.
cd ../..
B=$D/bin/bench

CFGS=baseline,native,yaegi,vm-native,vm-src,vm-mixed,vm-aot

# 1. Per-request cost + correctness: all configurations of a workload in one
#    process, measured round-robin (COUNT repetitions of ~BENCHTIME each).
for w in w1 w2 w3; do
	"$B" -w "$w" -cfg "$CFGS" -count "$COUNT" -benchtime "$BENCHTIME" \
		> "$OUT/bench-$w.tsv" 2> "$OUT/bench-$w.log"
done

# 2. Cold load time and memory: one fresh process per (workload, config).
: > "$OUT/load.tsv"
: > "$OUT/rss.tsv"
for w in w1 w2 w3; do
	for c in native yaegi vm-native vm-src vm-mixed vm-aot; do
		for rep in 1 2 3; do
			"$B" -w "$w" -cfg "$c" -count 1 -benchtime 100ms 2> "$OUT/load-$w-$c.log" |
				sed "s/^/$rep\t/" >> "$OUT/load.tsv"
			grep maxrss "$OUT/load-$w-$c.log" | sed "s/^/$w\t$c\t$rep\t/" >> "$OUT/rss.tsv"
		done
	done
done

# 3. CPU profiles of W2 in configurations 4 and 5.
for c in vm-src vm-mixed; do
	"$B" -w w2 -cfg "$c" -count 3 -benchtime 1s -cpuprofile "$OUT/w2-$c.pprof" > /dev/null 2>&1
	go tool pprof -top -nodecount=35 "$B" "$OUT/w2-$c.pprof" > "$OUT/w2-$c.top.txt" 2>/dev/null
done

# 4. Ablation: optimizations enabled cumulatively (VM_OPT disables a list).
if [ "${ABLATION:-1}" = 1 ]; then
	: > "$OUT/ablation.tsv"
	for step in "none" "-tuple,-fastnative,-memo,-bytes,-super" "-fastnative,-memo,-bytes,-super" \
		"-memo,-bytes,-super" "-bytes,-super" "-super" ""; do
		for w in w1 w2 w3; do
			VM_OPT="$step" "$B" -w "$w" -cfg yaegi,vm-native,vm-src,vm-mixed,vm-aot -count 5 -benchtime 200ms 2>/dev/null |
				sed "s/^/${step:-all}\t/" >> "$OUT/ablation.tsv"
		done
	done
fi

uptime >> "$OUT/env.txt"
echo "results in $OUT"
