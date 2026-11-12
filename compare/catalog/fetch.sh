#!/bin/sh
# fetch.sh: shallow-clones the repositories of plugins.txt into src/ (ignored),
# at their highest semver tag (the catalog serves tagged versions) or at the
# default branch when there is none, and records the version in src/COMMITS.tsv.
cd "$(dirname "$0")" || exit 1
mkdir -p src
: > src/COMMITS.tsv
export GIT_TERMINAL_PROMPT=0
grep -v '^#' plugins.txt | while read -r repo _; do
  d=src/$(echo "$repo" | tr / _)
  url=https://github.com/$repo
  if [ ! -d "$d/.git" ]; then
    tag=$(git ls-remote --tags --refs "$url" 2>/dev/null | sed 's|.*refs/tags/||' |
      grep -E '^v?[0-9]+\.[0-9]+\.[0-9]+$' | sort -t. -k1,1V -k2,2n -k3,3n | sort -V | tail -1)
    if [ -n "$tag" ]; then
      git clone -q --depth 1 --branch "$tag" "$url" "$d" </dev/null 2>/dev/null
    else
      git clone -q --depth 1 "$url" "$d" </dev/null 2>/dev/null
    fi
  fi
  if [ -d "$d/.git" ]; then
    v=$(git -C "$d" describe --tags --exact-match 2>/dev/null || echo HEAD)
    printf '%s\t%s\t%s\n' "$repo" "$v" "$(git -C "$d" rev-parse HEAD)" >> src/COMMITS.tsv
  else
    printf '%s\tCLONE-FAILED\t-\n' "$repo" >> src/COMMITS.tsv
  fi
done
