#!/bin/sh
# Regenerates aot/go1_NN/ (AOT copies for the Go release of the go command in
# use), natsyms/ (native symbol tables) and aot/go1_NN/REPORT.txt for the
# libraries evaluated in docs/AOTGEN.md. Run it once with the toolchain of
# each Go release of the window of yaegi v2 (internal/gorelease.Window).
set -e
cd "$(dirname "$0")/.."
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off
go build -o bin/aotgen ./cmd/aotgen
# encoding/json/v2 and jsontext exist only with GOEXPERIMENT=jsonv2: the
# default of Go 1.27, an experiment of Go 1.26.
jsonv2=
if go list encoding/json/v2 >/dev/null 2>&1; then
	jsonv2="encoding/json/v2 encoding/json/jsontext"
fi
# json-iterator reads runtime internals through reflect2 and cannot be
# copied: it stays native (docs/AOTGEN.md). Any other target that does not
# build makes aotgen fail.
./bin/aotgen -allow-fail github.com/json-iterator/go,github.com/modern-go/reflect2 "$@" \
	encoding/json $jsonv2 fmt errors sort encoding/xml encoding/gob encoding/binary encoding/asn1 \
	text/template html/template flag net/rpc log/slog database/sql \
	gopkg.in/yaml.v3 sigs.k8s.io/yaml github.com/goccy/go-yaml \
	github.com/BurntSushi/toml github.com/pelletier/go-toml/v2 \
	github.com/mitchellh/mapstructure github.com/go-viper/mapstructure/v2 \
	github.com/go-playground/validator/v10 github.com/json-iterator/go \
	github.com/spf13/cast github.com/google/go-cmp/cmp github.com/google/go-cmp/cmp/cmpopts
