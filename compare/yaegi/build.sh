#!/bin/sh
# build.sh: builds bin/govm (cmd/yaegi of this repository, under its former
# name: the scripts and results of this study use it) and bin/yrun (yaegi
# v0.16.1 configured as Traefik configures it, see yrun/main.go).
. "$(dirname "$0")/env.sh"
mkdir -p "$C/bin" "$C/out" "$C/emptygp/src"
(cd "$C/../.." && go build -o "$C/bin/govm" ./cmd/yaegi) || exit 1
(cd "$C/yrun" && go build -o "$C/bin/yrun" .) || exit 1
echo "built $C/bin/govm $C/bin/yrun"
