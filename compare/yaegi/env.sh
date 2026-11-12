# env.sh: sourced by the scripts of this directory. C is the directory of the
# study; binaries go to $C/bin (build.sh), outputs to $C/out (both ignored).
C=$(cd "$(dirname "$0")" && pwd)
export GOFLAGS=${GOFLAGS:--mod=mod}
export GOSUMDB=${GOSUMDB:-off}

# timed SECONDS CMD...: runs CMD in its own process group and kills the whole
# group after SECONDS (exit status 142, as SIGALRM). Unlike a plain alarm, it
# also stops the program that "go run" starts, and works when the caller
# ignores SIGALRM (background jobs).
timed() {
  perl -e 'my $t = shift; my $p = fork; if (!$p) { setpgrp(0, 0); exec @ARGV; exit 127 }
    $SIG{ALRM} = sub { kill "KILL", -$p; waitpid($p, 0); exit 142 }; alarm $t;
    waitpid($p, 0); exit($? & 127 ? 128 + ($? & 127) : $? >> 8)' "$@"
}
