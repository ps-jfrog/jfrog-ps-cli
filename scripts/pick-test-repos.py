#!/usr/bin/env python3
"""Pick N repos per package type that exist on BOTH servers, for testing
'jfps repos resolve-merge-diff'. Prints a semicolon-separated list, the format
'jf rt transfer-config-merge --include-repos' expects.

  jf api /artifactory/api/repositories --server-id=SRC 2>/dev/null > src.json
  jf api /artifactory/api/repositories --server-id=TGT 2>/dev/null > tgt.json
  python3 scripts/pick-test-repos.py src.json tgt.json [--per-type 2] [--offset 0]

Only repos with the same class (LOCAL/REMOTE/...) and package type on both
sides are considered; LOCAL repos are preferred. --offset skips that many
candidates per type, to test a fresh sample after an earlier run.
"""
import argparse, collections, json, sys


def load(path):
    text = open(path).read()
    start = text.index("[")
    # jf prints an "[Info] ..." line before the JSON; the JSON list starts "[ {" or "[{".
    for marker in ("[ {", "[{", "[]"):
        if marker in text:
            start = text.index(marker)
            break
    return {r["key"]: r for r in json.loads(text[start:])}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("source")
    ap.add_argument("target")
    ap.add_argument("--per-type", type=int, default=2)
    ap.add_argument("--offset", type=int, default=0)
    a = ap.parse_args()
    src, tgt = load(a.source), load(a.target)
    by_type = collections.defaultdict(list)
    for key in sorted(set(src) & set(tgt)):
        s, t = src[key], tgt[key]
        if s["type"] == t["type"] and s["packageType"] == t["packageType"]:
            by_type[s["packageType"]].append((s["type"] != "LOCAL", key))
    picked = []
    for pkg in sorted(by_type):
        chosen = [k for _, k in sorted(by_type[pkg])[a.offset:a.offset + a.per_type]]
        print(f"{pkg}: {len(by_type[pkg])} shared, picked {chosen}", file=sys.stderr)
        picked += chosen
    print(";".join(picked))


if __name__ == "__main__":
    main()
