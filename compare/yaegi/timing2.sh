#!/bin/sh
# timing2.sh: load+run time with a third-party dependency (semver), yaegi
# from a prepared GOPATH vs govm (go list + type-check + SSA).
. "$(dirname "$0")/env.sh"
G=$C/gp16
mkdir -p $G/src/github.com/Masterminds/semver
ln -sfn "$(go env GOMODCACHE)/github.com/!masterminds/semver/v3@v3.4.0" $G/src/github.com/Masterminds/semver/v3
cd $C/progs || exit 1
for i in 1 2 3 4 5; do
  perl -MTime::HiRes=time -e '$t=time; system(@ARGV); printf "yaegi l16 %.0fms\n", (time-$t)*1000' sh -c "$C/bin/yrun -gopath $G l16_semver.go >/dev/null 2>&1"
done
$C/bin/yrun -gopath $G l16_semver.go | cmp - $C/out/l16_semver.go.txt && echo "yaegi l16 output OK"
for i in 1 2 3 4 5; do
  perl -MTime::HiRes=time -e '$t=time; system(@ARGV); printf "govm l16 %.0fms\n", (time-$t)*1000' sh -c "$C/bin/govm l16_semver.go >/dev/null 2>&1"
done
$C/bin/govm -stats l16_semver.go 2>&1 >/dev/null | tail -1
