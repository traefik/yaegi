#!/bin/bash
set -xe

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <fuzz-type>"
    exit 1
fi

NAME=gojay
TYPE=$1

# Setup
export GO111MODULE="off"
go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build
go get -d -v -u ./...
wget -q -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/download/v2.4.29/fuzzit_Linux_x86_64
chmod a+x fuzzit

# Fuzz
go-fuzz-build -libfuzzer -o fuzzer.a ./interp
clang -fsanitize=fuzzer fuzzer.a -o fuzzer
./fuzzit create job $LOCAL --type $TYPE yaegi fuzzer
