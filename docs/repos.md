# `jfps repos`

Manages Artifactory repository lifecycle: listing repos on one server,
diffing/creating missing repos between a source and target server (never
deletes), syncing full repo configuration to bring an existing target repo
in line with source, and bulk-deleting repos from a file. `diff` and
`sync-config` always operate two-server (`--source-*`/`--target-*`);
`list`/`delete` are single-server.

| Subcommand | What it does |
|---|---|
| [`list`](#list) | List repositories on one server, optionally filtered by type |
| [`diff`](#diff) | Find repos missing on target vs. source, optionally create them |
| [`sync-config`](#sync-config) | Push source config to target for repos present on both, where they differ |
| [`delete`](#delete) | Bulk-delete repositories listed in a file |

---

## `list`

**What it does:** Lists repositories on a single server, optionally filtered by type.

**Key flags:** `--type` (default `all`) — local, remote, virtual, federated, or all

**Safety notes:** none (read-only)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps repos list
    participant Art as Artifactory

    User->>CLI: jfps repos list --type local
    CLI->>Art: GET /artifactory/api/repositories?type=local
    Art-->>CLI: repo list (key, type, url)
    CLI-->>User: print table
```

---

## `diff`

**What it does:** Finds repos present on source but missing on target, and — unless in dry-run — creates them on target by copying source's full config. Never deletes anything, and never touches repos that already exist on target.

**Key flags:** `--type` (required), `--dry-run` (default `true`), uses `--source-*`/`--target-*` server flags

**Safety notes:** dry-run defaults to true and is checked before any create call; there is no interactive confirmation even with `--dry-run=false`.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps repos diff
    participant Src as Source Artifactory
    participant Tgt as Target Artifactory

    User->>CLI: jfps repos diff --type local --dry-run=false
    CLI->>Src: GET /artifactory/api/repositories?type=local
    Src-->>CLI: source repo keys
    CLI->>Tgt: GET /artifactory/api/repositories?type=local
    Tgt-->>CLI: target repo keys
    Note over CLI: MissingOnTarget = source keys - target keys
    CLI-->>User: print missing-repo table
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop here
    else --dry-run=false
        loop each missing repo
            CLI->>Src: GET /artifactory/api/repositories/{key}
            Src-->>CLI: full config
            CLI->>Tgt: PUT /artifactory/api/repositories/{key}
            Tgt-->>CLI: created
        end
    end
```

---

## `sync-config`

**What it does:** For repos present on **both** servers, compares full configuration and pushes source's config to target wherever it differs. Only ever updates existing target repos — never creates or deletes.

**Key flags:** `--type` (required), `--dry-run` (default `true`), uses `--source-*`/`--target-*` server flags

**Safety notes:** dry-run defaults to true and gates the update calls; there is no interactive confirmation when disabled.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps repos sync-config
    participant Src as Source Artifactory
    participant Tgt as Target Artifactory

    User->>CLI: jfps repos sync-config --type local --dry-run=false
    CLI->>Src: GET /artifactory/api/repositories?type=local
    Src-->>CLI: source repo keys
    CLI->>Tgt: GET /artifactory/api/repositories?type=local
    Tgt-->>CLI: target repo keys
    Note over CLI: Common = source keys ∩ target keys
    loop each common key
        CLI->>Src: GET /artifactory/api/repositories/{key}
        Src-->>CLI: source config
        CLI->>Tgt: GET /artifactory/api/repositories/{key}
        Tgt-->>CLI: target config
        Note over CLI: deep-equal compare -> Differing?
    end
    CLI-->>User: print differing-repo table
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop here
    else --dry-run=false
        loop each differing key
            CLI->>Src: GET /artifactory/api/repositories/{key}
            Src-->>CLI: refetch source config
            CLI->>Tgt: POST /artifactory/api/repositories/{key}
            Tgt-->>CLI: updated
        end
    end
```

---

## `delete`

**What it does:** Bulk-deletes repositories whose keys are listed, one per line, in a file (blank lines and `#`-comments skipped).

**Key flags:** `--file` (required), `--dry-run` (default `true`), `--yes`

**Safety notes:** dry-run skips *all* HTTP calls, not just the deletes. When disabled, an interactive `y/N` confirmation is also required unless `--yes` is passed.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps repos delete
    participant Art as Artifactory

    User->>CLI: jfps repos delete --file repos.txt --dry-run=false
    Note over CLI: read repo keys from file (local, no HTTP)
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, no HTTP calls at all
    else --dry-run=false
        CLI->>User: confirm "Delete N repositories?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each repo key
            CLI->>Art: DELETE /artifactory/api/repositories/{key}
            Art-->>CLI: status / error
        end
    end
```
