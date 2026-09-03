# `jfps repos`

Manages Artifactory repository lifecycle: listing repos on one server,
diffing/creating missing repos between a source and target server (never
deletes), syncing full repo configuration to bring an existing target repo
in line with source, bulk-deleting repos from a file, checking whether a
list of repos exists, and creating/updating two-member virtual repos from a
CSV file. `diff` and `sync-config` always operate two-server
(`--source-*`/`--target-*`); `list`/`delete`/`check-exists`/`create-virtual`
are single-server.

| Subcommand | What it does |
|---|---|
| [`list`](#list) | List repositories on one server, optionally filtered by type |
| [`diff`](#diff) | Find repos missing on target vs. source, optionally create them |
| [`sync-config`](#sync-config) | Push source config to target for repos present on both, where they differ |
| [`delete`](#delete) | Bulk-delete repositories listed in a file |
| [`check-exists`](#check-exists) | Check whether repositories listed in a file exist on a server |
| [`create-virtual`](#create-virtual) | Create/update two-member (local+remote) virtual repos from a CSV file |

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

---

## `check-exists`

**What it does:** Checks whether repository keys listed, one per line, in a file exist on a single server. Reports EXISTS/NOT FOUND per key.

**Key flags:** `--file` (required), `--type` (default `all`) — local, remote, virtual, federated, or all

**Safety notes:** none (read-only). Exit code is `1` if any key is not found, so it's usable as a CI gate.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps repos check-exists
    participant Art as Artifactory

    User->>CLI: jfps repos check-exists --file repos.txt
    Note over CLI: read repo keys from file (local, no HTTP)
    CLI->>Art: GET /artifactory/api/repositories
    Art-->>CLI: full repo list
    Note over CLI: classify each key EXISTS / NOT FOUND
    CLI-->>User: print table + summary
```

---

## `create-virtual`

**What it does:** Creates or updates two-member virtual repositories (one local + one remote repo each) from a CSV file. For each row, validates that both member repos exist and share the same package type, then creates the virtual repo if it doesn't exist, updates it if its current members differ from the CSV, or reports "no-change" if they already match. Never touches the member repos themselves.

**Key flags:** `--file` (required CSV with `new-virtual-repo`, `local-name`, `remote-name` columns), `--dry-run` (default `true`), `--yes`

**Safety notes:** validation (member existence, package-type match, and — for already-existing virtual repos — a config fetch to detect whether an update is actually needed) always runs, even in dry-run; only the final create/update call is gated by `--dry-run` and, when disabled, an interactive confirmation (unless `--yes`).

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps repos create-virtual
    participant Art as Artifactory

    User->>CLI: jfps repos create-virtual --file virtual_repos.csv --dry-run=false
    Note over CLI: read CSV (local, no HTTP)
    CLI->>Art: GET /artifactory/api/repositories
    Art-->>CLI: full repo list (existence + packageType per key)
    loop each row
        alt virtual repo already exists
            CLI->>Art: GET /artifactory/api/repositories/{virtual-repo}
            Art-->>CLI: current members
            Note over CLI: members match CSV? no-change : update
        else virtual repo missing
            Note over CLI: plan = create
        end
    end
    CLI-->>User: print plan table (ACTION per row)
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop here
    else --dry-run=false
        CLI->>User: confirm "Create/update N virtual repo(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each create/update plan
            alt create
                CLI->>Art: PUT /artifactory/api/repositories/{virtual-repo}
            else update
                CLI->>Art: POST /artifactory/api/repositories/{virtual-repo}
            end
            Art-->>CLI: status / error
        end
    end
```
