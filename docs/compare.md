# `jfps compare`

Verifies migrations between two JPDs by diffing repository contents
(checksums), build-info (names/numbers, with optional migration), and
generic local JSON/config files.

**Safety model:** `repos` and `json` are strictly read-only (and `json`
isn't even server-connected). `builds` is read-only unless `--migrate
--dry-run=false` is passed together, in which case it uploads missing
build-info to target.

| Subcommand | What it does |
|---|---|
| [`repos`](#repos) | Compare artifact contents (checksums) between source and target |
| [`builds`](#builds) | Compare build names/numbers; optionally migrate missing ones |
| [`json`](#json) | Diff two local JSON files (no server involved) |

---

## `repos`

**What it does:** Compares artifact contents (path/name/sha256) of one or every local repo between a source and target JPD; source is treated as ground truth. Never modifies either server.

**Key flags:** `--repo`/`--all` (mutually exclusive), `--report-dir` (write per-repo `.txt` reports + `transfer-plan.txt` to disk instead of printing tables)

**Safety notes:** none (read-only)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps compare repos
    participant Src as Source Artifactory
    participant Tgt as Target Artifactory

    User->>CLI: jfps compare repos --all
    opt --all
        CLI->>Src: GET /artifactory/api/repositories?type=local
        Src-->>CLI: local repo keys
    end
    loop each repo
        CLI->>Src: POST /artifactory/api/search/aql (path/name/sha256)
        Src-->>CLI: source items
        CLI->>Tgt: POST /artifactory/api/search/aql (path/name/sha256)
        Tgt-->>CLI: target items
        Note over CLI: diff by key (local only, no HTTP)
    end
    alt --report-dir given
        Note over CLI: write per-repo .txt reports + transfer-plan.txt
    else
        CLI-->>User: print diff tables
    end
```

---

## `builds`

**What it does:** Compares build names/numbers between source and target. With `--migrate`, additionally uploads build-info for pairs missing on target — gated behind `--dry-run` like every other mutating command in this CLI.

**Key flags:** `--migrate`, `--dry-run` (default `true`)

**Safety notes:** mutating only via `--migrate`; defaults to `--dry-run=true`, so no upload happens unless the user explicitly passes `--dry-run=false`. No interactive confirmation prompt.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps compare builds
    participant Src as Source Artifactory
    participant Tgt as Target Artifactory

    User->>CLI: jfps compare builds --migrate --dry-run=false
    CLI->>Src: GET /artifactory/api/build
    Src-->>CLI: source build names
    CLI->>Tgt: GET /artifactory/api/build
    Tgt-->>CLI: target build names
    loop each shared build name
        CLI->>Src: GET /artifactory/api/build/{name}
        Src-->>CLI: build numbers
        CLI->>Tgt: GET /artifactory/api/build/{name}
        Tgt-->>CLI: build numbers
    end
    CLI-->>User: print name/number diff
    opt --migrate
        loop each source-only build name
            CLI->>Src: GET /artifactory/api/build/{name}
            Src-->>CLI: numbers (to build migration pair list)
        end
        alt dry-run (default)
            Note over CLI: print [DRY RUN] "would migrate" table, stop
        else --dry-run=false
            loop each missing (name, number) pair
                CLI->>Src: GET /artifactory/api/build/{name}/{number}
                Src-->>CLI: full build-info
                CLI->>Tgt: PUT /artifactory/api/build
                Tgt-->>CLI: build-info uploaded
            end
        end
    end
```

---

## `json`

**What it does:** Diffs two local JSON object files (added/removed/changed top-level keys). No server involved at all — purely local file reads and comparison.

**Key flags:** `--file1`, `--file2`

**Safety notes:** none (read-only, not server-connected)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps compare json

    User->>CLI: jfps compare json --file1 a.json --file2 b.json
    Note over CLI: read both files locally, parse as JSON objects
    Note over CLI: diff top-level keys: added / removed / changed
    CLI-->>User: print diff
```
