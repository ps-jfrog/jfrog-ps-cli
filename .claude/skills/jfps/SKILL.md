---
name: jfps
description: >-
  Use the jfps binary (this repo, jfrog-ps-cli) for JFrog Professional
  Services automation: repo/cleanup-policy/project discovery and HTML audit
  reporting, access (users/groups/permission targets) management, bulk
  cleanup (artifacts, repos, inactive users, stale Docker tags, remote-cache),
  repo/project sync between two JPDs, replication, federation, migration
  comparison (repo content, build-info, JSON diff), and Xray operations.
  Use this skill when the user wants to run jfps, build/install it, cut a
  release, or asks about any of: repo storage/activity/usage audit, cleanup
  policy coverage, "which repos have no cleanup policy", instance-wide
  discovery (versions, storage, auth/signing keys, users, groups,
  permissions, projects, replication, federation config), a self-contained
  HTML audit report, bulk repo/artifact/user deletion with dry-run, Docker
  tag cleanup, remote-cache cleanup gated on Xray indexing, or comparing two
  JFrog Platform Deployments during a migration.
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
Releases are cut by pushing a `v*` tag (see [Cutting a release](#cutting-a-release-maintainers)).

## Connecting to a JPD

Every subcommand takes the same connection flags, resolved in priority order
**explicit flags > saved profile (`--server <name>`) > environment
variables**:

```
--url <https://mycompany.jfrog.io>   or  JFPS_URL
--token <access-token>                or  JFPS_ACCESS_TOKEN
--user / --password (basic auth fallback, used only if --token is empty)
--insecure                            (skip TLS verification)
--server <name>                       (saved profile in ~/.jfps/servers.json)
```

Commands that reconcile two JPDs at once (`compare`, `replication restore`,
`repos diff`/`sync-config`, `projects diff`) use `--source-*`/`--target-*`
variants of every flag, and `JFPS_SOURCE_*`/`JFPS_TARGET_*` env vars.

There is no saved-profile `config add` subcommand yet — write
`~/.jfps/servers.json` directly (see the top-level README for the format).

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

## Command groups

| Command | Covers | Mutates? |
|---|---|---|
| `jfps discover` | One command, flags select the piece(s) (not subcommands): repo storage/activity/cleanup-policy/project reporting; instance-wide versions/storage/auth/signing-keys/users/groups/permissions/projects/replication/federation/platform-config/Xray-config; self-contained HTML audit | No — fully read-only |
| `jfps access` | Users, groups, permission targets: list/audit, create, grant-path, add/remove repos, backup, bulk delete | Yes (dry-run default) |
| `jfps cleanup` | AQL-driven artifact deletion, bulk repo deletion, inactive-user pruning, source-vs-target reconciliation, Docker tag cleanup, Xray-gated remote-cache cleanup | Yes (dry-run default) |
| `jfps repos` | List, diff (create-missing-only), full-config sync, bulk delete | List: no. Diff/sync/delete: yes |
| `jfps projects` | List, diff (create missing / optionally delete extra), bulk delete | List: no. Diff/delete: yes |
| `jfps replication` | List/check, create, delete, disable, backup, restore push replication | Yes (dry-run default) |
| `jfps federation` | Convert to/from federated, member health check, config resync | Yes (dry-run default) |
| `jfps compare` | Repo content diff (checksums), build-info diff/migrate, generic JSON diff | Diff: no. Migrate: yes |
| `jfps xray` | Index status/toggle/reindex, report list/delete, policy/watch/ignore-rule sync | Mixed — check subcommand |

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
progress bar to stderr - hand-rolled, no dependency, consistent with the
rest of this zero-dependency CLI.

`--policies` is the drill-down companion to `--repos`'s merged
`cleanup_policy_*` columns — one row per repo × matched policy (or a blank
row when nothing matches), for spotting uncovered or ambiguously-overlapping
repos that a single joined CSV cell would hide.

**Never captured, by design, even where the underlying API returns it:**
token/webhook secrets, license hashes, signing-key material (public or
private), proxy/mail-server passwords.

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

## Testing

```sh
go build ./...
go vet   ./...
go test  ./...   # mocked HTTP via httptest, no live JPD needed
```

Every package has real unit-test coverage (business-logic packages
typically 70–94%). There is no live-JPD integration suite — validate
against a real (ideally non-production) JPD before relying on this for
anything customer-facing.
