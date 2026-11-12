#!/bin/sh
# vendor.sh: fills the vendor/ directory of each plugin of plugins/ (the hosts
# of the catalog harness read only the plugin directory, as Traefik does).
# Needs the module cache or the network once.
cd "$(dirname "$0")/plugins" || exit 1
for d in */; do
  (cd "$d" && GOFLAGS=-mod=mod GOWORK=off go mod vendor) || echo "vendor failed: $d"
done
