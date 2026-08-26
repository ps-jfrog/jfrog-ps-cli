# jfps — JFrog Professional Services CLI

`jfps` is a single, dependency-free Go binary that consolidates the most
commonly reused JFrog Professional Services automation — access/permission
management, cleanup, repo & project sync, replication & federation,
migration verification (compare), repo/cleanup-policy discovery and HTML
audit reporting, and Xray operations — into one consistently-designed CLI,
generalized from the shell/Python scripts in the
[`ps_jfrog_scripts`](../ps_jfrog_scripts) repository (see that repo's
`docs/01`–`docs/06` for the scripts this project is based on).

It is a clean-room reimplementation, not a line-for-line port: every
subcommand below was designed against the REST endpoints and behavior
verified in those original scripts, but rebuilt with consistent flags,
real unit tests (mocked HTTP, no live JPD required), and one deliberate
safety change described below.

## Why this exists

The original scripts are hundreds of standalone shell/Python files with
inconsistent argument order, inconsistent auth handling (some Basic, some
Bearer, credentials often passed as plain CLI args), inconsistent (often
absent) dry-run support, and — in a few documented cases — real bugs
(delete calls silently commented out, substring-matching bugs, hardcoded
credentials, args shifted by a missing positional parameter). `jfps`
gives Professional Services engineers one binary with one connection
model and one safety model instead.

## Safety model (read this before running anything against production)

**Every mutating subcommand defaults `--dry-run` to `true`.** You must pass
`--dry-run=false` explicitly to make any change. In dry-run mode, mutating
commands issue **zero** write calls (POST/PUT/PATCH/DELETE) — only reads —
and print what they would have done. This is enforced by tests: every
mutating code path has a test that fails if the mock server's write handler
is ever invoked while `dryRun=true`.

The most destructive commands (bulk user/group/permission/repo/project
deletion) additionally require an interactive confirmation prompt even with
`--dry-run=false`, unless you pass `--yes` (for scripted/CI use).

## Building

```sh
cd jfrog-ps-cli
go build -o jfps ./cmd/jfps
```

No external dependencies — the whole tree is Go standard library only, so
this builds offline with any reasonably recent Go toolchain (developed and
tested against Go 1.26).

To cross-compile for every platform at once (linux/darwin/windows ×
amd64/arm64) via [goreleaser](https://goreleaser.com/) (`.goreleaser.yaml`):

```sh
goreleaser release --snapshot --clean   # -> dist/, untagged local build
```

## Testing

```sh
go build ./...   # whole module
go vet   ./...
go test  ./...   # all unit tests, mocked HTTP via httptest — no live JPD needed
```

Every package has real unit test coverage (business-logic packages
typically 70–90%). There is no live-JPD integration test suite in this
repo — validate against a real (ideally non-production) JPD before relying
on this for anything customer-facing, especially the more complex commands
(`federation convert-to-local`, `access recreate`-style flows).

## Connecting to a JPD

Every subcommand accepts the same connection flags, resolved in this
priority order: **explicit flags > saved profile (`--server <name>`) > environment
variables**.

```
--url <https://mycompany.jfrog.io>   or  JFPS_URL
--token <access-token>                or  JFPS_ACCESS_TOKEN
--user / --password (basic auth fallback, used if --token is empty)
--insecure                            (skip TLS verification)
--server <name>                       (a saved profile, see below)
```

Commands that talk to two JPDs at once (most `compare`, `replication`
`restore`, and the diff/sync commands) use `--source-*`/`--target-*`
variants of every flag above, and `JFPS_SOURCE_*`/`JFPS_TARGET_*`
environment variables.

### Saved server profiles

```sh
# There is no `config add` subcommand yet — write the profile file directly:
mkdir -p ~/.jfps
cat > ~/.jfps/servers.json <<'EOF'
{
  "servers": {
    "prod":  {"name": "prod",  "url": "https://mycompany.jfrog.io", "token": "..."},
    "stage": {"name": "stage", "url": "https://mycompany-stage.jfrog.io", "token": "..."}
  }
}
EOF

jfps repos list --server prod --type local
```

## Commands

Run `jfps` with no arguments, or `jfps <command>` with no subcommand, to see
the full list at any time. Every leaf subcommand also supports `-h`.

| Command | Covers |
|---|---|
| `jfps access` | Users, groups, permission targets: list/audit, create, grant-path, add/remove repos, backup, bulk delete with dry-run |
| `jfps cleanup` | AQL-driven artifact deletion (with built-in "not downloaded since" queries), bulk repo deletion, inactive-user detection/removal, source-vs-target artifact reconciliation, stale Docker/OCI tag cleanup, remote-cache cleanup gated on Xray index status |
| `jfps discover` | Per-repo storage/usage/activity/project + matched cleanup-policy detail (CSV); repo↔policy mapping; instance-wide versions/storage/auth/signing-keys/users/groups/permissions/projects/replication/federation (JSON); a self-contained interactive HTML audit combining all of it |
| `jfps repos` | List, diff (create-missing-only, never deletes), full-config sync, bulk delete |
| `jfps projects` | List, diff (create missing / optionally delete extra), bulk delete |
| `jfps replication` | List/check, create, delete, disable, backup, restore push replication |
| `jfps federation` | Convert to/from federated, member health check, config resync |
| `jfps compare` | Repo content diff (checksums), build-info diff/migrate, generic JSON diff |
| `jfps xray` | Index status/toggle/reindex, report list/delete, policy/watch/ignore-rule sync between JPDs |

See [`docs/`](docs/README.md) for a per-subcommand breakdown of what each
command does, its flags, and a sequence diagram of the HTTP calls it makes.

### A few examples

```sh
# List local repos
jfps repos list --url https://mycompany.jfrog.io --token $TOKEN --type local

# Preview (dry-run) which local repos are missing on a target JPD
jfps repos diff --source-url https://prod.jfrog.io --source-token $T1 \
                 --target-url https://dr.jfrog.io  --target-token $T2 --type local

# Actually create the missing ones
jfps repos diff ... --dry-run=false

# Find permission targets with no repos assigned
jfps access find-empty-permissions --url ... --token ...

# Preview bulk-deleting inactive users (>6 months, excluding internal realm)
jfps cleanup inactive-users --months 6 --url ... --token ...

# Preview artifacts never downloaded (or not downloaded in 6 months) -- no AQL required
jfps cleanup artifacts --repo my-repo --not-downloaded-since 6mo --url ... --token ...

# Preview deleting stale Docker tags -- deletes the whole tag, not just manifest.json
jfps cleanup docker-tags --repo docker-local --older-than 3mo --url ... --token ...

# Preview remote-cache cleanup: download-stale AND no longer Xray-indexed
jfps cleanup remote-cache --repo npm-remote --older-than 6mo --url ... --token ...

# Compare build-info between two JPDs after a migration
jfps compare builds --source-url ... --source-token ... --target-url ... --target-token ...

# Discover: per-repo storage/activity + matched cleanup-policy detail, scoped
# to a subset first on a large/shared instance (3 AQL calls per non-empty repo)
jfps discover repos --url ... --token ... --include 'myteam-*' --csv report.csv

# Discover instance-wide state: versions, storage, auth/signing keys, users,
# groups, permissions, projects, replication & federation config
jfps discover instance --url ... --token ... --output instance.json

# Turn both into one self-contained interactive HTML audit - no JPD
# connection needed for this step, works on files from any environment
jfps discover analyze --input report.csv --instance instance.json --output audit.html

# Check Xray federation/index status
jfps xray index status --url ... --token ... --type local
```

## Known limitations / assumptions worth knowing before you rely on this

A handful of API response shapes were not verified against a live JPD
(there was none available while building this) and were instead modeled
directly on what the equivalent original script parses. These are called
out in code comments at each call site, but the notable ones are:

- `cleanup inactive-users` assumes `GET /artifactory/api/security/users`
  returns a `lastLoggedIn` field; the exact field name/type should be
  confirmed against your JPD version before relying on it.
- `access backup`/`recreate`-style Access API v2 list responses
  (`/access/api/v2/users`, `/access/api/v2/groups`) are assumed
  non-paginated; large instances may need pagination added.
- `xray sync ignore-rules`'s project-scoping field is assumed to mirror
  `xray sync policies` (top-level `project_key`) rather than a confirmed
  schema.
- `xray reports list` fetches a single page (100 rows) — pagination isn't
  implemented yet.
- `cleanup remote-cache` treats any path missing from
  `POST /xray/api/v1/summary/artifact`'s `artifacts[]` (whether it appears in
  `errors[]` or is simply omitted) as "not indexed / retention expired." This
  mirrors the equivalent PS shell tool's classification, but the API's exact
  error text/shape for the "retention expired" case specifically (vs. "never
  indexed") was not re-verified against a live JPD here — treat the
  dry-run report as a starting point to spot-check before running with
  `--dry-run=false` on a production remote cache.
- `access recreate` (full backup-restore parity with the original Python
  tool's SCIM verification, `--force-actions`, and username-remapping
  modes) was intentionally not ported — this CLI's `access backup`/
  `delete-*` cover the safer, more commonly used half of that tool.

None of these are correctness bugs in the code as written — they're
places where "verify this endpoint shape against your actual JPD version
before trusting it in production" applies more than usual.

## Design notes

- Zero external dependencies (standard library only): `net/http` client in
  `internal/jfclient`, hand-rolled subcommand dispatch instead of a CLI
  framework, `flag.FlagSet` per subcommand.
- Each feature area is two packages: `internal/<area>` (pure business
  logic, unit-tested against `httptest` mock servers, no flag/env
  parsing) and `internal/cli/<area>` (thin flag-parsing/printing layer).
  This is what let this whole CLI be built as eight independent,
  concurrently-developed units that all compiled and passed tests on the
  very first attempt to wire them together.
- `internal/registry` is the seam between `cmd/jfps/main.go` and every
  feature package — each feature package self-registers via `init()`, so
  adding a new command group never means editing a shared switch
  statement.
