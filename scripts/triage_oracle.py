#!/usr/bin/env python3
"""Differential run of the triaged upstream-bug reproducers: go run vs yaegi vs the VM.

Usage: triage_oracle.py <corpus dir> <yaegi v0 binary> <yaegi v2 binary>

The corpus (one issue-<N>/ directory per triaged upstream issue, holding the
reproducer .go file and a result.json with "kind" and "status") comes from a
separate triage effort and is not part of this repository.

Output comparison: combined stdout+stderr, with panics reduced to their first
line, stack traces dropped, trailing whitespace stripped.
"""
import glob
import json
import os
import re
import subprocess
import sys

root, yaegi, vm = sys.argv[1], sys.argv[2], sys.argv[3]
env = dict(os.environ, GOFLAGS="-mod=mod", GOPROXY="off", GOSUMDB="off")


def run(cmd, cwd):
    try:
        p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=30, env=env)
    except subprocess.TimeoutExpired:
        return "TIMEOUT", None
    out = p.stdout + p.stderr
    lines = []
    for l in out.splitlines():
        if l.startswith("goroutine ") or l.startswith("\t") or l.startswith("exit status") or re.match(r"^\S+\.go:\d+:\d+: panic", l):
            continue
        if l.startswith("[signal"):
            continue
        lines.append(l.rstrip())
    norm = "\n".join(lines).strip()
    norm = re.sub(r"panic: (.*?)( \[recovered\])?\n.*", r"panic: \1", norm, flags=re.S)
    return norm, p.returncode


res = {}
for f in sorted(glob.glob(os.path.join(root, "issue-*/*.go"))):
    d, b = os.path.dirname(f), os.path.basename(f)
    issue = os.path.basename(d)
    meta = json.load(open(os.path.join(d, "result.json")))
    g, gc = run(["go", "run", b], d)
    if gc is None or (gc != 0 and "panic" not in g and "fatal error" not in g):
        res[issue] = ("GOFAIL", meta.get("kind"), meta.get("status"), "", "")
        continue
    y, yc = run([yaegi, "run", b], d)
    v, vc = run([vm, b], d)
    ys = "MATCH" if y == g and (yc == 0) == (gc == 0) else "DIFF"
    vs = "MATCH" if v == g and (vc == 0) == (gc == 0) else ("UNSUP" if vc == 3 else "DIFF")
    res[issue] = (ys, meta.get("kind"), meta.get("status"), vs, v.splitlines()[0][:110] if v and vs != "MATCH" else "")

for k, (ys, kind, st, vs, det) in sorted(res.items(), key=lambda x: int(x[0].split("-")[1])):
    print(f"{k:11s} kind={kind:12s} status={st:11s} yaegi={ys:6s} spikevm={vs:6s} {det}")
