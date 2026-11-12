#!/bin/sh
# race-shards.sh: runs the race detector on the VM in shards, so that CI runs
# them on several runners at once (one ./vm under -race took about 25
# minutes on one runner).
#
#   scripts/race-shards.sh run N    # shard N (1 to 4), with go test flags from $RACEFLAGS
#   scripts/race-shards.sh check    # each test of ./vm is in exactly one shard
#   scripts/race-shards.sh pattern N  # the -run pattern of shard N (the qemu
#                                     # jobs of CI use the same shards)
#
# Shards 1 to 4 split the tests of ./vm by name (the plugin tests, which
# load whole programs, are spread over two shards). Shard 1 also runs the
# other packages of ./vm/... and ./vreflect/.... "check" fails when a new
# test name matches no shard or two: then change the patterns below.
set -e
cd "$(dirname "$0")/.."

shard() {
	case $1 in
	1) echo '^Test[A-H]' ;;
	2) echo '^(Test[I-O]|TestP[a-km-z])' ;;
	3) echo '^TestPlugin[A-L]' ;;
	4) echo '^(TestPlugin[M-Z]|Test[Q-Z_a-z0-9]|Example|Fuzz)' ;;
	*) echo "no shard $1 (1 to 4)" >&2; exit 2 ;;
	esac
}

case $1 in
run)
	re=$(shard "$2")
	# shellcheck disable=SC2086
	go test -race -count=1 -timeout 30m $RACEFLAGS -run "$re" ./vm
	if [ "$2" = 1 ]; then
		others=$(go list ./vm/... ./vreflect/... | grep -v '/vm$')
		# shellcheck disable=SC2086
		go test -race -count=1 -timeout 30m $RACEFLAGS $others
	fi
	;;
pattern)
	shard "$2"
	;;
check)
	names=$(go test -race -list '.*' ./vm | grep -E '^(Test|Example|Fuzz)')
	bad=0
	for n in $names; do
		hits=0
		for i in 1 2 3 4; do
			if echo "$n" | grep -qE "$(shard $i)"; then
				hits=$((hits + 1))
			fi
		done
		if [ "$hits" != 1 ]; then
			echo "$n is in $hits race shards, want 1 (scripts/race-shards.sh)"
			bad=1
		fi
	done
	echo "$(echo "$names" | wc -l | tr -d ' ') tests of ./vm checked"
	exit $bad
	;;
*)
	echo "usage: $0 run N | pattern N | check" >&2
	exit 2
	;;
esac
