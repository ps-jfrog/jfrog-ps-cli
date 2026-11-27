# Resolve `transfer-config-merge` repository differences

## Use case

`jf rt transfer-config-merge` copies repositories that are missing on the
target, but for repositories that **already exist on both servers** and differ
it only writes a conflicts CSV (`transfer-config-conflicts-<timestamp>.csv`).
It never changes the target.

The differences are frequently unset-by-the-user flags whose default or
serialised value changed between Artifactory versions (for example `xrayIndex`,
`disableProxy`, `enableFileListsIndexing`, `handleSnapshots`, `xrayDataTtl`).
When the customer wants the target repos to match the source exactly,
`jfps repos resolve-merge-diff` does this:

1. Reads the `Repository` rows of the conflicts CSV (`Project` rows are ignored).
2. Fetches each repo's full config from source and target
   (`GET /artifactory/api/v2/repositories/<key>`, falling back to `api/repositories/<key>` on servers without v2) and writes a **before** report
   listing every differing property with its source and target value.
3. With `--dry-run=false`, updates the target with the **source value of only
   the differing properties** (partial `POST /artifactory/api/repositories/<key>`),
   then re-diffs and writes an **after** report proving the result.

Properties the merge command itself ignores (`url`, `password`, `description`,
`suppressPomConsistencyChecks`, `gitRegistryUrl`, `cargoInternalIndex`) and federated `members` (each JPD's own URL and
federation topology; copying it would re-point the target at the source) are
neither compared nor touched, so remote-repo URLs and credentials on the
target are preserved. Nothing is ever created or deleted.

### Why v2

`GET api/repositories/<key>` returns every legacy flag for every package type
(a Maven repo shows ~70 properties including Docker, NuGet, npm, Debian and
Cargo flags, whose defaults vary by Artifactory version). The v2 endpoint
returns only what applies to the repo's package type (25 for a Maven local
repo), so differences in irrelevant flags are not reported or pushed. Note that
`transfer-config-merge` itself uses the v1 endpoint, so some conflicts in its
CSV will show up as `match` here: they were noise.

## Usage

```
jfps repos resolve-merge-diff --csv <conflicts.csv> \
  --source-server-id <jf-server-id> --target-server-id <jf-server-id> \
  [--report-dir <dir>] [--dry-run=false]
```

| Flag | Meaning |
|---|---|
| `--csv` | Conflicts CSV from `jf rt transfer-config-merge` (required) |
| `--report-dir` | Where the report CSVs are written (default `.`) |
| `--dry-run` | Default `true`: only writes the before report. `--dry-run=false` applies updates |
| `--source-*` / `--target-*` | Standard server flags: `server-id`, `url`, `token`, `user`, `password`, `insecure`, `server` |

Use `--source-server-id`/`--target-server-id` to reuse servers already set up
with the JFrog CLI (`jf c show` lists them; `jf` must be on PATH). `--server`
reads a saved profile from `~/.jfps/servers.json`, which `jfps` has no command
to create, so it fails with `no saved server profile` unless you wrote that
file by hand.

Exit code is `0` only if every repo matches at the end; any update failure or
remaining difference exits `1`.

## Example

### 1. Preview (dry run, default)

```
$ jfps repos resolve-merge-diff \
    --csv testing/transfer-config-conflicts-2026-11-18.12-41-45.csv \
    --source-server-id old-rt --target-server-id new-rt --report-dir testing/report
Repositories in CSV: 249; with differences or errors: 249
Report: testing/report/resolve-merge-diff-before.csv
[DRY RUN] Would update 249 repositories on target. Re-run with --dry-run=false to apply.
```

`resolve-merge-diff-before.csv`:

```
Repository,Property,SourceValue,TargetValue,Status
airway-cosmo-vl-docker-dev-local,xrayIndex,true,false,differs
bcp-releases-local,enableFileListsIndexing,true,false,differs
```

Review it with the customer: each row is a change that will be made on the
target. A `match` row means the repo already agrees; an `error:` row means the
repo could not be read on one side (for example it was deleted).

### 2. Apply and verify

```
$ jfps repos resolve-merge-diff \
    --csv testing/transfer-config-conflicts-2026-11-18.12-41-45.csv \
    --source-server-id old-rt --target-server-id new-rt --report-dir testing/report \
    --dry-run=false
updated airway-cosmo-vl-docker-dev-local on target
...
After update: 0 of 249 repositories still differ or errored. Report: testing/report/resolve-merge-diff-after.csv
All differences resolved.
```

`resolve-merge-diff-after.csv` should contain only `match` rows. If any
`differs` rows remain, Artifactory rejected or normalised that value; check the
`failed to update ...` messages and inspect the repo in the UI.

### 3. Confirm with the original command

Re-run `jf rt transfer-config-merge` (without `--dry-run` side effects on
repos that now match); the repository rows in the new conflicts CSV should be
gone. `Project` rows (for example `storage_quota_bytes`, or the `jmgr`/`jfmgr`
key and name clashes) are not handled by this command and need manual review.

## Testing before a release

Run this before merging a change to `main` and tagging (for example `v1.9.1`).
It exercises the real release behaviour (release-only gates) against two real
servers: here `psazuse` (source) and `psemea` (target). **It updates repos on
the target**; use a non-production target, and keep the before report.

```bash
# 0. A release-like binary: any version other than "dev" turns on the release
#    gates (hidden commands, writes blocked unless explicitly opted in).
go build -ldflags "-X main.version=v1.9.1-rc1" -o /tmp/jfps-rc ./cmd/jfps
/tmp/jfps-rc --version                      # jfps version v1.9.1-rc1
/tmp/jfps-rc repos                          # lists only resolve-merge-diff
/tmp/jfps-rc repos sync-config -h           # must be refused (exit 1)

# 1. Pick N repos per package type that exist on both servers.
jf api /artifactory/api/repositories --server-id=psazuse 2>/dev/null > src.json
jf api /artifactory/api/repositories --server-id=psemea  2>/dev/null > tgt.json
REPOS=$(python3 scripts/pick-test-repos.py src.json tgt.json --per-type 2)
#   --offset 2 picks a fresh sample if an earlier run already aligned the first two.

# 2. Generate the conflicts CSV (reads only repos in the list; no projects).
jf rt transfer-config-merge psazuse psemea --include-repos "$REPOS" --exclude-projects "*"
#   Prints "Please review the report available at ~/.jfrog/logs/transfer-config-conflicts-<ts>.csv"
cp ~/.jfrog/logs/transfer-config-conflicts-<ts>.csv conflicts.csv

# 3. Dry run (default): writes the before report only.
/tmp/jfps-rc repos resolve-merge-diff --csv conflicts.csv \
  --source-server-id psazuse --target-server-id psemea --report-dir report

# 4. Review report/resolve-merge-diff-before.csv, then apply.
/tmp/jfps-rc repos resolve-merge-diff --csv conflicts.csv \
  --source-server-id psazuse --target-server-id psemea --report-dir report \
  --dry-run=false

# 5. Check report/resolve-merge-diff-after.csv.
grep -v ",match$" report/resolve-merge-diff-after.csv
```

**Pass criteria:** step 0 shows only `resolve-merge-diff` under `repos`;
step 4 prints `updated <repo> on target` lines and no `write/delete operations
are disabled in this build` error (that error would mean the write opt-in is
broken); and the after report shows `match` for every repo except the known
limitations below. Then merge and tag.

Results of this procedure on 2026-11-26 (31 repos, new sample, offset 2): 22 had
conflicts, 21 resolved, 1 remaining (`green-oci-dev-local`, empty `environments`).
An earlier run (46 repos, offset 0) resolved 27 of 32. Cases worth knowing:
a virtual repo's `repositories` list is also aligned (`sv-conan-virtual` lost
the member `sv-conan-remote`, as the source has only `sv-conan-local`), and a
remote repo's `curated`/`passThrough`/`listRemoteFolderItems` flags are pushed
as in the source.

## HTTP operations used

| Operation | Endpoint | When | Write? |
|---|---|---|---|
| GET | `/artifactory/api/v2/repositories/<key>` (source, target) | every run, before and after | no |
| GET | `/artifactory/api/repositories/<key>` | only if v2 returns 404 | no |
| POST | `/artifactory/api/repositories/<key>` (target only) | only with `--dry-run=false` | **yes** |

No PUT, PATCH or DELETE is used, and nothing is created or deleted. Release
builds block writes by default; this one POST (method and path prefix
`/artifactory/api/repositories/`) is the only exemption, so every other
mutating call in `jfps` stays blocked.

## Known limitations (seen in testing)

Verified with 46 repos (2 per package type) from `psazuse` to `psemea`: 32 had
conflicts, 27 resolved fully and the after report shows 5 remaining, for two reasons:

- **Missing prerequisite on the target.** `alexsh-generic-local` uses the
  property set `dmlAllowList`, which does not exist on the target, so
  Artifactory rejects the update (`HTTP 400: No property set found for
  property set: dmlAllowList`). Create the property set on the target and
  re-run. Property sets can be synced from source to target with the repo-sync
  plugin ([jfrog-cli-plugin-reposync](https://github.jfrog.info/JFROG/jfrog-cli-plugin-reposync)).
- **Empty `environments` is not applied.** When source has `environments: []`
  and the target has `["DEV"]`, the update returns success but Artifactory
  keeps the target's value. Clear these in the UI/Projects API if required.
  The after report keeps listing them as `differs` so they are not missed.

Re-running is safe: only repos that still differ are touched.

## Notes

- Released builds: this subcommand is promoted in `stableSubcommands`
  (`cmd/jfps/main.go`), so a downloaded release binary runs it; the rest of
  `jfps repos` is still dev-build-only. Its single write is opted out of the
  release write guard (`jfclient.AllowWrite`) and only after `--dry-run=false`.
- Keep the before report: it records the previous target values should a
  change need to be reverted.
- A standalone read-only diff generator for the same CSV lives in
  `jfrog-cli-core/testing/repoconfigdiff`.
