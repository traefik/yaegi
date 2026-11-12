#!/bin/sh
# run.sh prog.go [extra govm flags...]: runs a program under go run, yaegi
# (Traefik config), govm (default) and govm -aot; prints one status line.
. "$(dirname "$0")/env.sh"
P=$1; shift
N=$(basename "$P" .go)
D=$(cd "$(dirname "$P")" && pwd)
F=$D/$(basename "$P")
O=$C/out; mkdir -p $O
TO=${TO:-180}
run() { # mode cmd...
  m=$1; shift
  ( cd "$D" && timed $TO "$@" ) >$O/$N.$m.txt 2>&1
  echo $? >$O/$N.$m.rc
}
run go go run "$F"
run yaegi $C/bin/yrun ${YFLAGS:-} "$F"
run govm $C/bin/govm "$@" "$F"
run govmaot $C/bin/govm -aot "$@" "$F"
st() { # mode
  rc=$(cat $O/$N.$1.rc)
  if [ "$rc" = 0 ] && cmp -s $O/$N.go.txt $O/$N.$1.txt; then echo OK; return; fi
  case $1:$rc in
    *:0) echo "DIFF";;
    *:142|*:14) echo "TIMEOUT";;
    yaegi:2|govm*:2) echo "PANIC";;
    yaegi:3) echo "ERR";;
    govm*:3) echo "UNSUP";;
    *:4) echo "LOADERR";;
    *) if cmp -s $O/$N.go.txt $O/$N.$1.txt && [ "$rc" = "$(cat $O/$N.go.rc)" ]; then echo OK; else echo "EXIT$rc"; fi;;
  esac
}
printf "%-28s go=%s yaegi=%-8s govm=%-8s aot=%-8s\n" $N "$(cat $O/$N.go.rc)" $(st yaegi) $(st govm) $(st govmaot)
