#!/bin/sh
# deps.sh: for each measured plugin (results/catalog.tsv), lists the packages
# of its dependency graph that matter for AOT: non-standard packages that
# import reflect or unsafe, and standard packages that aot/ has a copy of.
# Output: results/deps.tsv (plugin, package, std, reflect, unsafe, in_aot).
cd "$(dirname "$0")" || exit 1
A=$(cd ../../aot && pwd)
PATH=$(go env GOROOT)/bin:$PATH
out=results/deps.tsv
mkdir -p results
printf 'plugin\tpackage\tstd\treflect\tunsafe\tin_aot\n' > $out
awk -F'\t' 'NR>1 && $3=="" {print $1}' results/catalog.tsv | while read -r repo; do
  d=src/$(echo "$repo" | tr / _)
  imp=$(sed -n 's/^import:[[:space:]]*//p' "$d/.traefik.yml" | tr -d "\"' \r")
  # module of the package: the nearest go.mod above its directory
  mod=$(sed -n 's/^module[[:space:]]*//p' "$d/go.mod" | tr -d '"\r')
  md=$d/${imp#"$mod"}
  while [ "$md" != "$d" ] && [ ! -f "$md/go.mod" ]; do md=$(dirname "$md"); done
  gf=-mod=mod; [ -d "$md/vendor" ] && gf=-mod=vendor
  (cd "$md" && GOFLAGS=$gf GOPROXY=off GOWORK=off GOTOOLCHAIN=auto go list -deps \
     -f '{{.ImportPath}} {{.Standard}} {{join .Imports " "}}' "$imp" 2>/dev/null) |
  while read -r pkg std imps; do
    case " $imps " in *" reflect "*) r=1;; *) r=0;; esac
    case " $imps " in *" unsafe "*) u=1;; *) u=0;; esac
    a=0; [ -d "$A/$pkg" ] && a=1
    if [ "$std" = true ]; then
      [ $a = 1 ] || continue
    else
      [ $r = 0 ] && [ $u = 0 ] && continue
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$repo" "$pkg" "$std" $r $u $a >> $out
  done
done
