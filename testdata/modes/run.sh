#!/bin/bash
# Runs the reflect battery of testdata/modes/cases in each mode and
# compares stdout and the exit code with the compiled program (go build,
# the same as go run). check.sh runs it for default mode and AOT mode.
#
# usage: testdata/modes/run.sh -out DIR [-timeout 60] mode=command ...
#   each mode is a name and a yaegi command line, for example
#   v1=/path/yaegi  v1aot="/path/yaegi -aot"
#
# Verdicts, per case and mode:
#   PASS     same stdout and same exit code as the compiled program
#   WRONG    runs to the end with the exit code of the compiled program,
#            stdout differs (a silent error)
#   REFUSED  load error (exit 4), construct not supported (exit 3), or a
#            panic of the engine that says it does not support something
#   CRASH    any other failure: a panic that the compiled program does not
#            have, a fatal error, a signal, a timeout
# The file DIR/verdicts.tsv holds: case, mode, verdict, a short note.
#
# Other platforms: with GOARCH=arm64 in the environment, the cases are
# built for arm64 and NATIVE_RUNNER (for example qemu-aarch64) runs them;
# the mode commands then name the arm64 yaegi v2 under the same runner.
set -u
cd "$(dirname "$0")"
out=""
timeout=60
while [ $# -gt 0 ]; do
	case "$1" in
	-out) out=$2; shift 2 ;;
	-timeout) timeout=$2; shift 2 ;;
	*) break ;;
	esac
done
[ -n "$out" ] || { echo "usage: $0 -out DIR mode=command ..." >&2; exit 2; }
mkdir -p "$out"
: >"$out/verdicts.tsv"

# timeout(1) is coreutils: macOS does not have it. gtimeout (brew install
# coreutils) or perl (every Mac has it) does the same: the command is
# killed after N seconds, and the exit code is 124.
if ! command -v timeout >/dev/null 2>&1; then
	if command -v gtimeout >/dev/null 2>&1; then
		timeout() { gtimeout "$@"; }
	else
		timeout() {
			perl -e '
				my $t = shift;
				my $pid = fork;
				if ($pid == 0) { exec @ARGV or exit 127 }
				$SIG{ALRM} = sub { kill KILL => $pid; waitpid $pid, 0; exit 124 };
				alarm $t;
				waitpid $pid, 0;
				exit(($? & 127) ? 128 + ($? & 127) : $? >> 8);
			' "$@"
		}
	fi
fi

engine_refusal='not supported|unsupported|no stub|refused|cannot be|not in registry|vm: method|has no reflect|not implemented'

for dir in cases/*/; do
	c=$(basename "$dir")
	# CASES (a regular expression) limits the run to some cases.
	[[ -z "${CASES:-}" || "$c" =~ ${CASES:-} ]] || continue
	mkdir -p "$out/$c"
	go build -o "$out/$c/native" "./$dir" 2>"$out/$c/build.err" || { echo "$c: go build failed" >&2; continue; }
	# NATIVE_RUNNER runs the compiled program of another GOARCH (qemu-aarch64).
	# shellcheck disable=SC2086
	timeout "$timeout" ${NATIVE_RUNNER:-} "$out/$c/native" >"$out/$c/go.out" 2>"$out/$c/go.err"
	echo $? >"$out/$c/go.rc"
	for spec in "$@"; do
		m=${spec%%=*}
		cmd=${spec#*=}
		# shellcheck disable=SC2086
		timeout "$timeout" $cmd "./$dir" >"$out/$c/$m.out" 2>"$out/$c/$m.err"
		rc=$?
		echo $rc >"$out/$c/$m.rc"
		exp=$(cat "$out/$c/go.rc")
		first=$(grep -v '^\s*$' "$out/$c/$m.err" | head -1 | cut -c1-160 | tr '\t' ' ')
		if cmp -s "$out/$c/go.out" "$out/$c/$m.out" && [ "$rc" = "$exp" ]; then
			v=PASS
			first=""
		elif [ "$rc" = "$exp" ]; then
			v=WRONG
			first="$(diff "$out/$c/go.out" "$out/$c/$m.out" | grep -c '^[<>]') diff lines"
		elif [ "$rc" = 124 ]; then
			v=CRASH
			first="timeout ${timeout}s"
		elif [ "$rc" = 3 ] || [ "$rc" = 4 ]; then
			v=REFUSED
		elif grep -Eq 'fatal error:|SIGSEGV|SIGBUS|unexpected signal|unexpected fault address' "$out/$c/$m.err" || [ "$rc" -gt 128 ]; then
			v=CRASH
		elif grep -Eiq "$engine_refusal" "$out/$c/$m.err"; then
			v=REFUSED
		else
			v=CRASH
		fi
		# Was stdout already wrong before the failure?
		if [ "$v" = REFUSED ] || [ "$v" = CRASH ]; then
			# tr: wc of macOS pads the count with spaces, and its head
			# refuses -c 0 (an empty stdout is never wrong).
			n=$(wc -c <"$out/$c/$m.out" | tr -d ' ')
			if [ "$n" -gt 0 ] && ! head -c "$n" "$out/$c/go.out" | cmp -s - "$out/$c/$m.out"; then
				first="$first [stdout wrong before the stop]"
			fi
		fi
		printf '%s\t%s\t%s\t%s\n' "$c" "$m" "$v" "$first" >>"$out/verdicts.tsv"
		printf '%-26s %-6s %-8s %s\n' "$c" "$m" "$v" "$first"
	done
done
