#!/bin/sh
# setup.sh: prepares a fresh clone (local or cloud session). Needs the network
# once: afterwards every tool works with GOPROXY=off.
#   - downloads the modules of yaegi v2, of the comparison tools (compare/,
#     a module of its own that requires yaegi v0) and of the yaegi v0 study;
#   - builds and vets everything, unless the generated data is for another
#     Go release (scripts/release-check.sh): then it says what to regenerate.
set -e
cd "$(dirname "$0")/.."

go version
# go.mod and go.sum (root and compare/) are tidy (CI checks it): -mod=readonly
# fails if they are not complete, instead of changing them in silence. The
# comparison study modules under compare/yaegi are not gated and keep -mod=mod.
export GOFLAGS=-mod=readonly
# The build lists (not "all": test-only dependencies of dependencies are not
# needed), then everything the packages and their tests import.
go mod download
go list -deps -test ./... >/dev/null
(cd compare && go mod download && go list -deps -test ./... >/dev/null)
(cd compare/yaegi/progs && GOFLAGS=-mod=mod go mod download && GOFLAGS=-mod=mod go list -deps ./... >/dev/null)
(cd compare/yaegi/yrun && GOFLAGS=-mod=mod go mod download && GOFLAGS=-mod=mod go list -deps ./... >/dev/null)

# The generated data is for the Go releases of the window of yaegi v2
# (internal/gorelease.Window). With another release (a new Go, or the CI job
# "next Go"), its release_guard.go files stop the build on purpose: the setup
# then stops before the build, so the regeneration can run.
rc=0
sh scripts/release-check.sh || rc=$?
if [ "$rc" = 2 ]; then
	echo "setup ok (modules), not built: regenerate the data first (docs/DEVELOPMENT.md, \"Following a new Go release\"):"
	echo "  go generate ./stdtypes ./stdsyms ./vm && sh scripts/aotgen.sh"
	echo "then: go build ./... && go vet ./..."
	exit 0
elif [ "$rc" != 0 ]; then
	exit "$rc"
fi

# One command per line: with "set -e", a failure on the left of && does not
# stop the script.
go build ./...
go vet ./...
(cd compare && go build ./... && go vet ./...)
echo "setup ok: see README.md and TODO.md"
