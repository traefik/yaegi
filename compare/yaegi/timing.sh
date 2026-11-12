#!/bin/sh
# timing.sh prog.go...: wall time (ms) of whole runs, 5 times each, sequential.
. "$(dirname "$0")/env.sh"
cd $C/progs || exit 1
wall() {
  perl -MTime::HiRes=time -e '$t=time; system(@ARGV); printf "%.0f\n", (time-$t)*1000' "$@" 2>/dev/null
}
for f in "$@"; do
  for mode in yaegi govm govm-aot gorun; do
    ts=""
    for i in 1 2 3 4 5; do
      case $mode in
        yaegi) t=$(wall sh -c "$C/bin/yrun -gopath $C/emptygp $f >/dev/null 2>&1");;
        govm) t=$(wall sh -c "$C/bin/govm $f >/dev/null 2>&1");;
        govm-aot) t=$(wall sh -c "$C/bin/govm -aot $f >/dev/null 2>&1");;
        gorun) t=$(wall sh -c "go run $f >/dev/null 2>&1");;
      esac
      ts="$ts $t"
    done
    echo "$f $mode ms:$ts"
  done
  $C/bin/govm -stats $f 2>&1 >/dev/null | tail -1
  $C/bin/govm -aot -stats $f 2>&1 >/dev/null | tail -1
done
