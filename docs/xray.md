# `jfps xray`

Manages three areas of Xray operation: repository indexing (check which
repos are Xray-indexed, toggle indexing on/off, trigger re-index/rescan),
scan-report lifecycle (list/filter reports, bulk-delete old ones by
end-time cutoff), and cross-JPD synchronization of security policies,
watches, and ignore rules. Indexing and reports operate against a single
server; `sync` operates against a source/target pair.

**Safety model:** all mutating actions default `--dry-run=true`. `reports
delete` additionally requires interactive confirmation unless `--yes` is
passed; the other mutating actions (`index toggle`, `index reindex`, `sync
*`) have no confirmation gate beyond dry-run.

| Subcommand | What it does |
|---|---|
| [`index status`](#index-status) | Show which repos of a type are Xray-indexed |
| [`index toggle`](#index-toggle) | Enable/disable Xray indexing on one repo or a type |
| [`index reindex`](#index-reindex) | Trigger (re)indexing/rescanning of a repo |
| [`reports list`](#reports-list) | List Xray reports, optionally filtered |
| [`reports delete`](#reports-delete) | Bulk-delete reports older than a cutoff |
| [`sync policies`](#sync-policies) | Copy every security policy from source to target |
| [`sync watches`](#sync-watches) | Copy every watch from source to target |
| [`sync ignore-rules`](#sync-ignore-rules) | Copy every ignore rule from source to target |

---

## `index status`

**What it does:** Shows which repositories of a given type are Xray-indexed.

**Key flags:** `--type` (default `local`)

**Safety notes:** none (read-only)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray index status
    participant Art as Artifactory
    participant Xray

    User->>CLI: jfps xray index status --type local
    CLI->>Xray: GET /xray/api/v1/binMgr/default/repos
    Xray-->>CLI: already-indexed repo keys
    CLI->>Art: GET /artifactory/api/repositories?type=local
    Art-->>CLI: repos of that type
    Note over CLI: merge in memory
    CLI-->>User: print indexed/not-indexed table
```

---

## `index toggle`

**What it does:** Enables or disables Xray indexing on one repo or every repo of a type.

**Key flags:** `--repo`/`--all` (mutually exclusive; `--type` required with `--all`), `--enable`/`--disable`, `--dry-run` (default `true`)

**Safety notes:** dry-run default; no interactive confirmation gate even at `--dry-run=false`.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray index toggle
    participant Art as Artifactory

    User->>CLI: jfps xray index toggle --all --type local --enable --dry-run=false
    opt --all
        CLI->>Art: GET /artifactory/api/repositories?type=local
        Art-->>CLI: repo keys
    end
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop
    else --dry-run=false
        loop each repo
            CLI->>Art: POST /artifactory/api/repositories/{repo} {"xrayIndex": true}
            Art-->>CLI: updated
        end
    end
```

---

## `index reindex`

**What it does:** Triggers (re)indexing or rescanning of a single repo, in one of three modes: `index` (standard, adds a newly-indexed repo), `force` (full re-scan, needs Xray Admin, can be expensive), or `scan-now` (also needs Xray Admin, also expensive on large repos).

**Key flags:** `--repo` (required), `--mode` = `index`\|`force`\|`scan-now` (required), `--dry-run` (default `true`)

**Safety notes:** dry-run default; no `--yes`/confirmation prompt — be deliberate with `force`/`scan-now` on large repos.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray index reindex
    participant Xray

    User->>CLI: jfps xray index reindex --repo R --mode force --dry-run=false
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner describing the call, stop
    else --dry-run=false
        alt mode=index
            CLI->>Xray: POST /xray/api/v1/index/repository/R
        else mode=force
            CLI->>Xray: POST /xray/api/v1/forceReindex {"repos":["R"], is_non_indexed_repo_allowed:true}
        else mode=scan-now
            CLI->>Xray: POST /xray/api/v2/index {"repos":["R"]}
        end
        Xray-->>CLI: triggered
    end
```

---

## `reports list`

**What it does:** Lists Xray reports, optionally filtered by status/type. Fetches a single page (100 rows) — no pagination loop yet.

**Key flags:** `--status`, `--report-type`

**Safety notes:** none (read-only)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray reports list
    participant Xray

    User->>CLI: jfps xray reports list --status completed
    CLI->>Xray: POST /xray/api/v1/reports {page_number:1, num_of_rows:100, filters:{...}}
    Xray-->>CLI: reports (first page only)
    CLI-->>User: print table
```

---

## `reports delete`

**What it does:** Bulk-deletes reports whose `end_time` is before a given RFC3339 cutoff (`--status`/`--report-type` further narrow the candidate list; reports with an unparsable `end_time` are skipped, not deleted).

**Key flags:** `--status`, `--report-type`, `--end-time` (required, RFC3339), `--dry-run` (default `true`), `--yes`

**Safety notes:** dry-run default; additionally gated by interactive confirmation unless `--yes` is passed.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray reports delete
    participant Xray

    User->>CLI: jfps xray reports delete --end-time 2025-01-01T00:00:00Z --dry-run=false
    CLI->>Xray: POST /xray/api/v1/reports {filters:{...}}
    Xray-->>CLI: candidate reports
    Note over CLI: split into matches (end_time < cutoff) vs skipped (unparsable)
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner + match table, stop
    else --dry-run=false
        CLI->>User: confirm "Delete N report(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each matching report
            CLI->>Xray: DELETE /xray/api/v1/reports/{id}
            Xray-->>CLI: deleted
        end
    end
```

---

## `sync policies`

**What it does:** Copies every Xray security policy from a source JPD to a target JPD. Project-scoped policies are posted with `?projectKey=<key>` and have `project_key` stripped from the body; global policies are posted plainly.

**Key flags:** `--dry-run` (default `true`), uses `--source-*`/`--target-*` connection flags

**Safety notes:** dry-run default; no interactive confirmation/`--yes` gate.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray sync policies
    participant Src as Source Xray
    participant Tgt as Target Xray

    User->>CLI: jfps xray sync policies --dry-run=false
    CLI->>Src: GET /xray/api/v2/policies
    Src-->>CLI: all policies
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop
    else --dry-run=false
        loop each policy
            alt global
                CLI->>Tgt: POST /xray/api/v2/policies
            else project-scoped
                CLI->>Tgt: POST /xray/api/v2/policies?projectKey={key}
            end
            Tgt-->>CLI: created
        end
    end
```

---

## `sync watches`

**What it does:** Copies every Xray watch from source to target, same project-scoping rules as `sync policies` (nested `general_data.project_key` is stripped for project-scoped watches).

**Key flags:** `--dry-run` (default `true`), uses `--source-*`/`--target-*` connection flags

**Safety notes:** dry-run default; no confirmation/`--yes` gate.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray sync watches
    participant Src as Source Xray
    participant Tgt as Target Xray

    User->>CLI: jfps xray sync watches --dry-run=false
    CLI->>Src: GET /xray/api/v2/watches
    Src-->>CLI: all watches
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop
    else --dry-run=false
        loop each watch
            alt global
                CLI->>Tgt: POST /xray/api/v2/watches
            else project-scoped
                CLI->>Tgt: POST /xray/api/v2/watches?projectKey={key}
            end
            Tgt-->>CLI: created
        end
    end
```

---

## `sync ignore-rules`

**What it does:** Copies every Xray ignore rule from source to target, same project-scoping pattern. The project-key handling for this specific endpoint is a documented, unverified assumption (mirrors `sync policies`'s shape rather than a confirmed schema).

**Key flags:** `--dry-run` (default `true`), uses `--source-*`/`--target-*` connection flags

**Safety notes:** dry-run default; no confirmation/`--yes` gate.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps xray sync ignore-rules
    participant Src as Source Xray
    participant Tgt as Target Xray

    User->>CLI: jfps xray sync ignore-rules --dry-run=false
    CLI->>Src: GET /xray/api/v1/ignore_rules
    Src-->>CLI: all ignore rules
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, stop
    else --dry-run=false
        loop each rule
            alt global
                CLI->>Tgt: POST /xray/api/v1/ignore_rules
            else project-scoped
                CLI->>Tgt: POST /xray/api/v1/ignore_rules?projectKey={key}
            end
            Tgt-->>CLI: created
        end
    end
```
