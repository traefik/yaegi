#!/bin/sh
. "$(dirname "$0")/env.sh"
cd $C/progs || exit 1
timed 180 $C/bin/govm "$@"
