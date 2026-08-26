# `jfps projects`

Manages JFrog Projects (Access API v1): listing them on one server, diffing
two servers' project sets to create-missing/optionally-delete-extra on a
target, and bulk-deleting projects by key from a file.

| Subcommand | What it does |
|---|---|
| [`list`](#list) | List all projects on one server |
| [`diff`](#diff) | Create projects missing on target; optionally delete extras on target |
| [`delete`](#delete) | Bulk-delete projects listed in a file |

---

## `list`

**What it does:** Lists all JFrog Projects on a single server.

**Safety notes:** none (read-only)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps projects list
    participant JPD

    User->>CLI: jfps projects list
    CLI->>JPD: GET /access/api/v1/projects
    JPD-->>CLI: all projects
    CLI-->>User: print PROJECT_KEY/DISPLAY_NAME table
```

---

## `diff`

**What it does:** Compares projects between a source and target server. Creates projects missing on target (unless dry-run). If `--delete-extra` is also given, additionally deletes target-only projects.

**Key flags:** `--dry-run` (default `true`), `--delete-extra`, `--yes`

**Safety notes:** `--dry-run` gates both creates and deletes; deleting extras additionally requires `--delete-extra` plus (`--yes` or interactive confirmation).

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps projects diff
    participant Src as Source JPD
    participant Tgt as Target JPD

    User->>CLI: jfps projects diff --delete-extra --dry-run=false
    CLI->>Src: GET /access/api/v1/projects
    Src-->>CLI: source projects
    CLI->>Tgt: GET /access/api/v1/projects
    Tgt-->>CLI: target projects
    Note over CLI: diff by project_key -> Missing / Extra
    CLI-->>User: print Missing-on-target / Extra-on-target

    alt Missing on target
        alt dry-run (default)
            Note over CLI: print [DRY RUN] banner, stop
        else --dry-run=false
            loop each missing project
                CLI->>Src: GET /access/api/v1/projects/{key}
                Src-->>CLI: config
                CLI->>Tgt: POST /access/api/v1/projects
                Tgt-->>CLI: created
            end
        end
    end

    alt Extra on target AND --delete-extra
        alt dry-run (default)
            Note over CLI: print [DRY RUN] banner, stop
        else --dry-run=false
            CLI->>User: confirm "Delete N extra project(s)?" [y/N]
            User-->>CLI: y (or --yes was passed)
            loop each extra project
                CLI->>Tgt: DELETE /access/api/v1/projects/{key}
                Tgt-->>CLI: deleted
            end
        end
    end
```

---

## `delete`

**What it does:** Bulk-deletes projects whose keys are listed, one per line, in a file.

**Key flags:** `--file` (required), `--yes`

**Safety notes:** there is no `--dry-run` flag on this subcommand by design (mirrors the original script's "confirm once, then do it") — an interactive confirmation (or `--yes`) is mandatory before any DELETE is issued, so nothing is deleted without at least one explicit gate.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps projects delete
    participant JPD

    User->>CLI: jfps projects delete --file projects.txt
    Note over CLI: read project keys from file (local, no HTTP)
    CLI->>User: confirm "Delete N project(s)?" [y/N]
    alt declined
        Note over CLI: abort, no HTTP calls at all
    else confirmed (or --yes was passed)
        loop each project key
            CLI->>JPD: DELETE /access/api/v1/projects/{key}
            JPD-->>CLI: deleted / error
        end
    end
```
