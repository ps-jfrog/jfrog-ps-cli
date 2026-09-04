---
name: jfps
description: >-
  Use the jfps binary (this repo, jfrog-ps-cli) for JFrog Professional
  Services automation: repo/cleanup-policy/project discovery and HTML audit
  reporting, access (users/groups/permission targets/instance backup-restore)
  management, bulk cleanup (artifacts, repos, inactive users, empty folders,
  stale Docker tags, remote-cache), repo/project sync between two JPDs,
  replication, federation, migration comparison (repo content, build-info,
  JSON diff), and Xray operations (indexing, JAS toggle, report generation/
  segregation, policy/watch/ignore-rule sync).
  Use this skill when the user wants to run jfps, build/install it, cut a
  release, or asks about any of: repo storage/activity/usage audit, cleanup
  policy coverage, "which repos have no cleanup policy", instance-wide
  discovery (versions, storage, auth/signing keys, users, groups,
  permissions, projects, replication, federation config), a self-contained
  HTML audit report, bulk repo/artifact/user deletion with dry-run, Docker
  tag cleanup, remote-cache cleanup gated on Xray indexing, backing up/
  restoring users-groups-permissions, generating/segregating Xray reports, or
  comparing two JFrog Platform Deployments during a migration.
  Do NOT use this for general single-artifact JFrog CLI operations (upload,
  download, build-publish, AQL search, GraphQL/OneModel queries) - that's the
  `jf` CLI and its own `jfrog` skill, a different tool. jfps wraps bulk/
  reporting/PS-workflow operations the `jf` CLI doesn't do directly.
metadata:
  role: tool
  repo: jfrog-ps-cli
---

# jfps — JFrog Professional Services CLI

`jfps` is a single, dependency-free Go binary (this repo) consolidating
JFrog PS automation into one consistently-designed CLI. It is **not** the
`jf` CLI — don't confuse the two. `jfps` talks to Artifactory/Access/Xray
over plain HTTP (its own `internal/jfclient`), independent of any `jf`
config or session.

## Building / getting a binary

```sh
go build -o jfps ./cmd/jfps          # local build, reports "dev" for -v
goreleaser release --snapshot --clean # -> dist/, all 6 platform/arch combos
```

No external Go dependencies — builds offline with any recent Go toolchain.
Releases are cut by pushing a `v*` tag (see [Version / cutting a release](#version--cutting-a-release-maintainers)).

## Connecting to a JPD

Every subcommand takes the same connection flags, resolved in this priority
order: **explicit flags > saved profile (`--server <name>`) > a server
already configured in the JFrog CLI (`--server-id <id>`) > jfps env vars >
the JFrog CLI's own ad hoc env vars**:

```
--url <https://mycompany.jfrog.io>   or  JFPS_URL      or  JF_URL / JFROG_URL
--token <access-token>                or  JFPS_ACCESS_TOKEN  or  JF_ACCESS_TOKEN / JFROG_ACCESS_TOKEN
--user / --password (basic auth fallback, used only if --token is empty)
--insecure                            (skip TLS verification)
--server <name>                       (saved jfps profile in ~/.jfps/servers.json)
--server-id <id>                      (reuse a server already added via 'jf config add'/'jf login';
                                        requires the 'jf' binary on PATH. JFROG_CLI_SERVER_ID works
                                        the same way, for single-server commands)
```

Commands that reconcile two JPDs at once (`compare`, `replication restore`,
`repos diff`/`sync-config`, `projects diff`) use `--source-*`/`--target-*`
variants of every flag (including `--source-server-id`/`--target-server-id`),
and `JFPS_SOURCE_*`/`JFPS_TARGET_*` env vars. `JFROG_CLI_SERVER_ID` and the
unprefixed `JF_*`/`JFROG_*` fallbacks only apply to single-server commands —
the JFrog CLI has no source/target equivalents.

There is no saved-profile `config add` subcommand yet — write
`~/.jfps/servers.json` directly (see the top-level README for the format).
`--server-id` needs no such file: it shells out to `jf config export <id>`
to pull the URL/token from a server the user already set up with the JFrog
CLI.

## Safety model — read before running anything mutating

**Every mutating subcommand defaults `--dry-run` to `true`.** In dry-run
mode, zero write calls (POST/PUT/PATCH/DELETE) are issued — only reads —
and the command prints what it would have done. You must pass
`--dry-run=false` explicitly to apply a change. The most destructive
commands (bulk user/group/permission/repo/project deletion) additionally
require an interactive `[y/N]` confirmation even with `--dry-run=false`,
unless `--yes` is passed.

**When helping a user run jfps: default to dry-run.** Only pass
`--dry-run=false` when the user has explicitly asked to apply the change,
not merely to "check" or "see" something.

**A second, release-wide safety net sits underneath all of that**, independent
of any per-command `--dry-run` flag:

- **Writes are disabled by default at the HTTP layer**
  (`jfclient.WritesDisabled = true`) for any binary built with a version
  string other than `"dev"` — i.e. every tagged release. Every PUT/PATCH/
  DELETE, and POST except a handful of known read-only endpoints (AQL
  search, Xray artifact-summary, Xray report list/generate), fails with
  `operation not supported` regardless of `--dry-run=false` or `--yes`. A
  plain local `go build ./cmd/jfps` (reports `-v` as `dev`) re-enables
  writes, so untested commands can still be exercised for real locally.
- **Tagged release binaries only show/run commands that have been fully
  tested against a real JPD.** `cmd/jfps/main.go`'s `stableCommands` map is
  the allowlist — as of this writing it's just `{"discover": true}`. Every
  other command group is still fully built into the binary and runs fine in
  a dev build, but a tagged binary hides it from `jfps help`/`jfps -h` and
  refuses to run it (`jfps: "access" is not available in this release yet
  (still under test)`). Promoting a command means adding it to
  `stableCommands`, once someone has actually verified it end-to-end.

Net effect: **a downloaded release binary today can only run `discover`**
(itself read-only, so the writes gate never even matters for it). To use
any other command group — `access`, `cleanup`, `repos`, `projects`,
`replication`, `federation`, `compare`, `xray` — build from source
(`go build -o jfps ./cmd/jfps`) rather than downloading a release asset.

## Command groups

| Command | Covers | Mutates? | Stable in a tagged release? |
|---|---|---|---|
| `jfps discover` | One command, flags select the piece(s) (not subcommands): repo storage/activity/cleanup-policy/project reporting; instance-wide versions/storage/auth/signing-keys/users/groups/permissions/projects/replication/federation/platform-config/Xray-config; self-contained HTML audit | No — fully read-only | **Yes** |
| `jfps access` | Users, groups, permission targets: list/audit/effective-permissions, create, grant-path, add/remove repos, backup, restore (from a backup dir, additive-only), bulk delete | Yes (dry-run default) | No (dev builds only) |
| `jfps cleanup` | AQL-driven artifact deletion, bulk repo deletion, inactive-user pruning, empty-folder pruning, source-vs-target reconciliation, Docker tag cleanup, Xray-gated remote-cache cleanup | Yes (dry-run default) | No |
| `jfps repos` | List, check-exists, diff (create-missing-only), full-config sync, create-virtual (from a CSV spec), bulk delete | List/check-exists: no. Diff/sync/create-virtual/delete: yes | No |
| `jfps projects` | List, diff (create missing / optionally delete extra), bulk delete | List: no. Diff/delete: yes | No |
| `jfps replication` | List/check, create, delete, disable, backup, restore push replication | Yes (dry-run default) | No |
| `jfps federation` | Convert to/from federated, member health check, config resync | Yes (dry-run default) | No |
| `jfps compare` | Repo content diff (checksums), build-info diff/migrate, generic JSON diff | Diff: no. Migrate: yes | No |
| `jfps xray` | Index status/toggle/reindex, JAS/Contextual-Analysis toggle, report generation (date-chunked, CSV export, deployer/build-gitrepo segregation, upload-to-repo)/list/delete, policy/watch/ignore-rule sync | Mixed — check subcommand | No |

"Stable in a tagged release" means it's in `cmd/jfps/main.go`'s
`stableCommands` allowlist (see [Safety model](#safety-model--read-before-running-anything-mutating)
above) — everything else still works fully in a **dev build**
(`go build -o jfps ./cmd/jfps`), just not in a downloaded release binary yet.

Run `jfps <command>` with no subcommand, or `jfps <command> <subcommand> -h`,
for the exact flags (`jfps discover -h` for that one - it has no
subcommands, just selection flags, see below). Full per-command flag/behavior reference:
[`docs/`](../../../docs/README.md) (one page per command, with a sequence
diagram of the actual HTTP calls each subcommand makes).

## `discover` — the read-only reporting command

This is the command to reach for whenever the ask is "audit," "report on,"
"what repos/users/policies exist," or "give me a dashboard" — not "clean
up" or "delete" (that's `cleanup`/`access`). It is entirely read-only; the
analyze step makes no network calls at all.

**Not subcommands — one command, boolean selection flags**, so one
invocation can do several pieces together:

```sh
jfps discover --repos --instance --analyze --url ... --token ...
# or the shorthand for all four:
jfps discover --all --url ... --token ...
```

`--repos`/`--policies`/`--instance` run **concurrently** by default when more
than one is selected (independent network calls against the same JPD -
`--parallel=false` for sequentially, with full progress bars back instead of
the suppressed-while-concurrent ones); `--analyze` always runs last, so it
picks up whatever `--repos`/`--instance` just wrote (via
`--repos-csv`/`--instance-json`, both with sane defaults).

**Output paths:** left at their defaults, `--repos-csv`/`--policies-csv`/
`--instance-json`/`--audit-html` all land under `--output` (default
`output`), in a subfolder named after the JPD's hostname prefix - e.g.
`--url https://psazuse.jfrog.io` writes `output/psazuse/report.csv`, so
repeated runs against different instances never collide. Passing any of
those four flags explicitly overrides that one path completely, ignoring
`--output` for it.

**Output format choice, and why:** `--repos`/`--policies` write **CSV** (flat
per-row data — opens directly in Excel/Sheets, and it's what `--analyze`
parses). `--instance` writes **JSON**, not YAML — it's machine-generated
hierarchical data (nested users/groups/permissions/policies), and JSON avoids
YAML's implicit-typing gotchas (`no`/`yes` becoming booleans, a version
string like `3.10` becoming a number) that a hand-rolled parser would
otherwise have to guard against.

Run individually - each prints a "Next:" hint block to stderr telling you
exactly this:

```sh
# 1. Per-repo storage/activity + cleanup-policy detail + project assignment
jfps discover --repos --url ... --token ... --repos-csv report.csv
#    On a large/shared instance, scope first: --include 'team-a-*'
#    (3 AQL calls per non-empty repo - be considerate)

# 2. Instance-wide: versions, storage, auth/signing keys, users, groups,
#    permission targets, projects, replication, federation, platform config,
#    Xray policy/watch/ignore-rule/curation config
jfps discover --instance --url ... --token ... --instance-json instance.json

# 3. Combine into one self-contained interactive HTML audit (no network calls)
jfps discover --analyze --repos-csv report.csv --instance-json instance.json --audit-html audit.html
```

`--analyze` works with just a repos CSV (a single "Repos" tab: findings +
explorer). Adding an instance snapshot adds eight more tabs, in this order,
with **Overview as the default active tab** (Repos is the default only
without instance data) and the instance's URL shown top-right in the report
header: Overview (the instance's name and URL, versions, storage, an
**Environment counts** section - projects, repos, users, groups, permission
targets, federated repos, replication configs, global roles - plus auth
methods and a **highlighted callout if repository signing keys are
configured**, metadata only, never key material), then Repos, Projects
(repo counts; click a row for repos/roles/environments/members), Access (searchable
Users/Groups/Permission-Targets tables), Federation & Replication, Platform
Config (roles/tokens/certificates/**signing keys**/property-sets/proxies/
backups/archive-cleanup-policies/workers/webhooks, plus mail-server/
password-policy/lockout-policy/reverse-proxy/license/trash-retention as
stat cards), Xray (policies/watches/ignore-rules/curation policies/settings
— shows a plain notice instead if Xray isn't licensed), **All Findings**
(every flagged resource, of any type, in one searchable table — open this
one first for "what's wrong across the whole environment"), and
**Relationships** (project-to-repo view: pick Global or a project, with a
search box to narrow the project list — a hub-and-spoke SVG diagram plus a
full searchable table of that project's repos, or every repo with no
project for Global; the direct answer to "which repos belong to this
project").

`--repos` findings (computed by `--analyze`, each skipped rather than
mis-flagged if its source column is absent): `empty`, `stale` (no
push/pull inside `--stale-days`, default 180), `huge`/`massive-files` (over
`--huge-gb`/`--massive-files`), `naming-nonconformant`/`uppercase`/
`underscore`/`very-long-name`, `no-cleanup-policy`, `multi-policy-match`.

Cross-resource findings on the **All Findings** tab (instance data only):
`no-principals`/`wildcard-scope` (permission targets), `empty-project`,
`single-member` (a federated repo with <2 enabled members), `disabled` (a
configured-but-disabled replication), `non-expiring` (a scoped token with
no expiry), `inactive` (an Xray watch that's off), `no-rules` (an Xray
policy with no rules configured).

Every concurrent-bounded stage (`--repos`' AQL sweep; `--instance`'s
permission targets, project detail, federated repos) prints a live text
progress bar to stderr, via the shared `internal/progress`/`internal/
concurrency` machinery every other bulk command uses too — see
[Progress bars & concurrency](#progress-bars--concurrency-every-bulk-command) below.

`--policies` is the drill-down companion to `--repos`'s merged
`cleanup_policy_*` columns — one row per repo × matched policy (or a blank
row when nothing matches), for spotting uncovered or ambiguously-overlapping
repos that a single joined CSV cell would hide.

**Never captured, by design, even where the underlying API returns it:**
token/webhook secrets, license hashes, signing-key material (public or
private), proxy/mail-server passwords.

## Progress bars & concurrency (every bulk command)

Every subcommand that loops over many independent items — bulk deletes,
syncs, creates, restores, comparisons, across every command group, not just
`discover` — shows a live text progress bar on stderr (`internal/progress`;
falls back to a single `label: N/N` completion line when stderr isn't a
terminal) and runs those items with up to 8 in flight at once
(`internal/concurrency`'s `Map`/`One`/`MapOrErr` helpers), instead of one
request at a time. This is transparent to callers — same flags, same
output shape, just faster wall-clock time on anything with more than a
handful of items.

A handful of loops are deliberately left sequential where concurrency would
be unsafe rather than just slower: `xray reports generate`'s per-date-chunk
report generation (each chunk is an expensive server-side job; running many
at once risks overloading Xray's report engine) and the build-gitrepo
segregation step of that same command (it shares an unsynchronized
build-attribution cache across iterations). If you're adding a new bulk
loop, match the existing pattern — an independent-item worker function
passed to `concurrency.One`/`Map`/`MapOrErr`, never a loop touching shared
mutable state without its own synchronization — and validate with
`go test -race`.

## Version / cutting a release (maintainers)

```sh
jfps -v   # or --version / version -> "jfps version v1.2.3" (or "dev" for a plain go build)
```

Pushing a `v*` tag triggers `.github/workflows/release.yml`: `go vet`/
`go test`, then goreleaser builds all 6 platform/arch binaries (linux/
darwin/windows × amd64/arm64) and publishes a GitHub Release. The tag is
baked into the binary via `-X main.version={{.Tag}}` in `.goreleaser.yaml`,
so `-v` always matches the tag exactly:

```sh
git tag v1.2.3
git push origin v1.2.3
```

Before tagging, check whether any newly-tested command should move from
"dev build only" to generally available: add it to the `stableCommands` map
in `cmd/jfps/main.go`. Nothing else needs to change — the command's code is
already fully built into every binary; that map only controls whether a
tagged release shows/runs it (see [Safety model](#safety-model--read-before-running-anything-mutating)).

## Testing

```sh
go build ./...
go vet   ./...
go test  ./...          # mocked HTTP via httptest, no live JPD needed
go test -race ./...     # required for anything touching a bulk/concurrent loop
```

Every package has real unit-test coverage (business-logic packages
typically 70–94%). There is no live-JPD integration suite — validate
against a real (ideally non-production) JPD before relying on this for
anything customer-facing. Always run the `-race` variant after touching a
bulk operation or `internal/concurrency`/`internal/progress` themselves —
it has caught real races in this codebase before (both in production code
and in test fixtures that assumed sequential execution).
