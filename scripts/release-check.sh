#!/bin/sh
# release-check.sh: checks that the Go release of the go command in use (and
# its GOFLAGS tags) is in the window of the generated data (stdtypes,
# stdsyms, aot/all: internal/gorelease.Window). With another release, the
# release_guard.go of these packages is part of the build and stops it on
# purpose.
#
# Exit status: 0 when the data is for this release; 2, with the names of the
# packages, when it is for another release (regenerate first:
# docs/DEVELOPMENT.md, "Following a new Go release"); any other status is an error of go list.
set -e
cd "$(dirname "$0")/.."
stale=$(go list -f '{{range .GoFiles}}{{if eq . "release_guard.go"}}{{$.ImportPath}}{{"\n"}}{{end}}{{end}}' ./stdtypes ./stdsyms ./aot/all)
if [ -n "$stale" ]; then
	echo "these packages were generated for another Go release than $(go env GOVERSION) and do not build:"
	echo "$stale"
	exit 2
fi
