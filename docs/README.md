# jfps Documentation

Per-command reference for `jfps`, the JFrog Professional Services CLI. Each
page below covers one command group: what every subcommand does, its key
flags, its safety behavior, and a sequence diagram of the actual HTTP calls
it makes against Artifactory/Access/Xray, in order.

See the top-level [README](../README.md) for installation, connection
flags, and the CLI's overall safety model.

## Command groups

| Command | Covers |
|---|---|
| [`access`](access.md) | Users, groups, permission targets: list/audit, create, grant-path, add/remove repos, backup, bulk delete |
| [`cleanup`](cleanup.md) | AQL-driven artifact deletion, bulk repo deletion, inactive-user pruning, source-vs-target reconciliation, Docker tag cleanup, Xray-gated remote-cache cleanup |
| [`discover`](discover.md) | One command with selection flags (not subcommands): repo storage/activity/cleanup-policy/project reporting; instance-wide versions/storage/auth/signing-keys/users/groups/permissions/projects/replication/federation/platform-config/Xray-config; a self-contained interactive HTML audit combining all of it |
| [`repos`](repos.md) | List, diff (create-missing-only), full-config sync, bulk delete |
| [`projects`](projects.md) | List, diff (create missing / optionally delete extra), bulk delete |
| [`replication`](replication.md) | List/check, create, delete, disable, backup, restore push replication |
| [`federation`](federation.md) | Convert to/from federated, member health check, config resync |
| [`compare`](compare.md) | Repo content diff (checksums), build-info diff/migrate, generic JSON diff |
| [`xray`](xray.md) | Index status/toggle/reindex, report list/delete, policy/watch/ignore-rule sync |

## Reading the diagrams

Every diagram uses the same conventions:

- **`alt dry-run (default) / --dry-run=false`** — every mutating subcommand
  in this CLI defaults `--dry-run` to `true`. The dry-run branch always
  shows exactly what stops there: no write call is issued.
- **A confirm step (`CLI->>User: confirm ... [y/N]`)** — appears only on
  the subcommands that require it; it's skipped automatically when `--yes`
  is passed.
- **Two participants for source/target** — commands that reconcile two
  servers (most `compare`, `replication restore`, the `diff`/`sync`
  commands, `xray sync *`) show both explicitly; single-server commands
  show one.
- Diagrams show the **typical successful path**. Error handling (a failed
  call short-circuits with a printed error and non-zero exit code) is
  described in prose above each diagram where it materially changes
  behavior (e.g. `federation convert-to-local`'s fail-safe abort).

## Safety model recap

Every mutating subcommand across every group defaults `--dry-run` to
`true`; in dry-run mode zero write calls (POST/PUT/PATCH/DELETE) are
issued, only reads. The most destructive commands (bulk user/group/
permission/repo/project/report deletion) additionally require an
interactive confirmation once `--dry-run=false` is passed, unless `--yes`
is given for scripted/CI use. See each command's page for exactly which
subcommands carry that extra confirmation gate and which don't.
