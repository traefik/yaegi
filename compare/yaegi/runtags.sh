#!/bin/sh
# runtags.sh name...: re-runs govm (default and -aot) with pure-Go build tags
# (what yaegi selects implicitly, its build context not being "gc").
. "$(dirname "$0")/env.sh"
cd $C/progs || exit 1
TAGS=${TAGS:-purego,appengine,nounsafe,safe}
for n in "$@"; do
  f=$(/bin/ls $n*.go)
  for a in "" "-aot"; do
    o=$C/out/$n.govmtags$a.txt
    timed 180 $C/bin/govm $a -tags $TAGS $f >$o 2>&1
    rc=$?
    if [ $rc = 0 ] && cmp -s $o $C/out/$n*.go.txt; then s=OK; else s="rc=$rc $(head -c 220 $o | tr '\n' '|')"; fi
    echo "$n tags$a: $s"
  done
done
