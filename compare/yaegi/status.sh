#!/bin/sh
# status.sh: recomputes the status of every program from out/ (no rerun).
. "$(dirname "$0")/env.sh"
O=$C/out
st() { # name mode
  rc=$(cat $O/$1.$2.rc)
  if [ "$rc" = 0 ] && cmp -s $O/$1.go.txt $O/$1.$2.txt; then echo OK; return; fi
  case $2:$rc in
    *:0) echo DIFF;;
    *:142|*:14) echo TIMEOUT;;
    yaegi:2|govm*:2) echo PANIC;;
    yaegi:3) echo ERR;;
    govm*:3) echo UNSUP;;
    *:4) echo LOADERR;;
    *) echo "EXIT$rc";;
  esac
}
for f in $C/progs/*.go; do
  N=$(basename $f .go)
  [ -f $O/$N.go.rc ] || continue
  printf "%-38s go=%s yaegi=%-7s govm=%-7s aot=%s\n" $N "$(cat $O/$N.go.rc)" $(st $N yaegi) $(st $N govm) $(st $N govmaot)
done
