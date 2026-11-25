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
  --source-server <profile> --target-server <profile> \
  [--report-dir <dir>] [--dry-run=false]
```

| Flag | Meaning |
|---|---|
| `--csv` | Conflicts CSV from `jf rt transfer-config-merge` (required) |
| `--report-dir` | Where the report CSVs are written (default `.`) |
| `--dry-run` | Default `true`: only writes the before report. `--dry-run=false` applies updates |
| `--source-*` / `--target-*` | Standard server flags: `server`, `server-id`, `url`, `token`, `user`, `password`, `insecure` |

Exit code is `0` only if every repo matches at the end; any update failure or
remaining difference exits `1`.

## Example

### 1. Preview (dry run, default)

```
$ jfps repos resolve-merge-diff \
    --csv testing/transfer-config-conflicts-2026-11-18.12-41-45.csv \
    --source-server old-rt --target-server new-rt --report-dir testing/report
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
    --source-server old-rt --target-server new-rt --report-dir testing/report \
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

- Dev builds only: like other write commands, this is enabled in `dev` builds
  (`go build ./cmd/jfps`); tagged releases keep writes disabled.
- Keep the before report: it records the previous target values should a
  change need to be reverted.
- A standalone read-only diff generator for the same CSV lives in
  `jfrog-cli-core/testing/repoconfigdiff`.
