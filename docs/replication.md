# `jfps replication`

Manages Artifactory push replication configuration end-to-end against a
single JPD (there's no cross-server source/target pair for most
subcommands — `create` just writes a target URL into the payload it sends
to that one server). Covers discovery (`list`), setup (`create`), teardown
(`delete`, `disable`), and config portability (`backup`/`restore` to/from a
local JSON file).

| Subcommand | What it does |
|---|---|
| [`list`](#list) | Show whether push replication is configured, and its target(s) |
| [`create`](#create) | Create push replication from one or more repos to a target JPD/repo |
| [`delete`](#delete) | Remove push replication config from one repo or all configured repos |
| [`disable`](#disable) | Turn off (not delete) push replication |
| [`backup`](#backup) | Save replication config to a local JSON file |
| [`restore`](#restore) | Re-apply a replication config previously saved by `backup` |

---

## `list`

**What it does:** Shows whether push replication is configured for one repo or all local repos, and its target URL(s). A non-2xx response for a repo is treated as "not configured," not an error.

**Key flags:** `--repo` (default: all local repos)

**Safety notes:** none (read-only)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps replication list
    participant JPD

    User->>CLI: jfps replication list
    opt --repo not given
        CLI->>JPD: GET /artifactory/api/repositories?type=local
        JPD-->>CLI: local repo keys
    end
    loop each repo
        CLI->>JPD: GET /artifactory/api/replications/{repo}
        JPD-->>CLI: config, or non-2xx = "not configured"
    end
    CLI-->>User: print table
```

---

## `create`

**What it does:** Creates push replication from one or more source repos to a target JPD/repo, on a cron schedule.

**Key flags:** `--repo`/`--repo-file` (one required), `--target-*` connection flags (embedded in the payload), `--target-repo` (default: same key), `--cron` (default `0 0/6 * * * ?`), `--sync-deletes` (default `false`), `--enabled` (default `true`), `--dry-run` (default `true`)

**Safety notes:** dry-run defaults to true and skips all PUT calls; there is no interactive confirmation even with `--dry-run=false`.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps replication create
    participant JPD

    User->>CLI: jfps replication create --repo r1 --target-url ... --dry-run=false
    Note over CLI: build replication payload locally per repo
    alt dry-run (default)
        Note over CLI: print payload, no HTTP calls made
    else --dry-run=false
        loop each repo
            CLI->>JPD: PUT /artifactory/api/replications/{repo}
            JPD-->>CLI: created/updated
        end
    end
```

---

## `delete`

**What it does:** Removes push replication configuration from one repo or every configured local repo.

**Key flags:** `--repo`/`--all` (one required), `--dry-run` (default `true`), `--yes`

**Safety notes:** dry-run defaults to true; when disabled, an interactive confirmation is required unless `--yes` is passed.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps replication delete
    participant JPD

    User->>CLI: jfps replication delete --all --dry-run=false
    opt --all
        CLI->>JPD: GET /artifactory/api/repositories?type=local
        JPD-->>CLI: local repo keys
    end
    loop each candidate
        CLI->>JPD: GET /artifactory/api/replications/{repo}
        JPD-->>CLI: config, or "not configured" (excluded)
    end
    CLI-->>User: print target list
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop
    else --dry-run=false
        CLI->>User: confirm [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each confirmed target
            CLI->>JPD: DELETE /artifactory/api/replications/{repo}
            JPD-->>CLI: deleted
        end
    end
```

---

## `disable`

**What it does:** Turns off (but does not delete) push replication for one repo or every configured local repo, by rewriting `enabled: false` into the existing config.

**Key flags:** `--repo`/`--all` (one required), `--dry-run` (default `true`)

**Safety notes:** dry-run defaults to true; unlike `delete`, there is **no `--yes`/confirmation gate** — disabling is treated as lower-risk/reversible.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps replication disable
    participant JPD

    User->>CLI: jfps replication disable --all --dry-run=false
    opt --all
        CLI->>JPD: GET /artifactory/api/repositories?type=local
        JPD-->>CLI: local repo keys
    end
    loop each candidate
        CLI->>JPD: GET /artifactory/api/replications/{repo}
        JPD-->>CLI: config, or "not configured" (excluded)
    end
    CLI-->>User: print target list
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop
    else --dry-run=false
        loop each target
            CLI->>JPD: GET /artifactory/api/replications/{repo}
            JPD-->>CLI: current config
            Note over CLI: set enabled=false
            CLI->>JPD: PUT /artifactory/api/replications/{repo}
            JPD-->>CLI: updated
        end
    end
```

---

## `backup`

**What it does:** Saves the raw replication config of one or more repos to a local JSON file for later `restore`. Read-only against the server — only a local file is written.

**Key flags:** `--output` (required), `--repo-file` (default: all local repos)

**Safety notes:** none (read-only against the server)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps replication backup
    participant JPD

    User->>CLI: jfps replication backup --output backup.json
    opt no --repo-file
        CLI->>JPD: GET /artifactory/api/repositories?type=local
        JPD-->>CLI: local repo keys
    end
    loop each candidate
        CLI->>JPD: GET /artifactory/api/replications/{repo}
        JPD-->>CLI: config, or "not configured" (excluded)
    end
    loop each configured target
        CLI->>JPD: GET /artifactory/api/replications/{repo}
        JPD-->>CLI: raw config body
    end
    Note over CLI: write backup.json (local file only)
```

---

## `restore`

**What it does:** Re-applies replication configs previously saved by `backup`.

**Key flags:** `--input` (required), `--dry-run` (default `true`)

**Safety notes:** dry-run defaults to true; no interactive confirmation gate when disabled.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps replication restore
    participant JPD

    User->>CLI: jfps replication restore --input backup.json --dry-run=false
    Note over CLI: read + parse local JSON file (no HTTP)
    alt dry-run (default)
        Note over CLI: print repos that would be restored, no HTTP calls
    else --dry-run=false
        loop each repo (sorted by key)
            CLI->>JPD: PUT /artifactory/api/replications/{repo}
            JPD-->>CLI: restored
        end
    end
```
