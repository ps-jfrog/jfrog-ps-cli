# `jfps cleanup`

Bulk-deletion and reconciliation commands: AQL-driven artifact deletion
(with built-in "not downloaded since" query building), bulk repository
deletion, inactive-user detection/removal, source-vs-target artifact
reconciliation, stale Docker/OCI tag cleanup, and remote-repository cache
cleanup gated on Xray index status.

**Safety model:** every subcommand here mutates state, and every one of them
defaults `--dry-run` to `true`. In dry-run mode, zero write calls (POST/PUT/
DELETE) are issued — only reads. The most destructive subcommands
(`repos`, `inactive-users`, `docker-tags`, `remote-cache`) additionally
require an interactive `y/N` confirmation once `--dry-run=false` is passed,
unless `--yes` is also given.

| Subcommand | What it does |
|---|---|
| [`artifacts`](#artifacts) | Delete artifacts matched by an AQL query (hand-written, or built from `--repo`/`--not-downloaded-since`) |
| [`repos`](#repos) | Delete repositories listed in a file |
| [`inactive-users`](#inactive-users) | Find/delete users inactive for N+ months |
| [`diff-delete-artifacts`](#diff-delete-artifacts) | Delete target-only artifacts so target matches source |
| [`docker-tags`](#docker-tags) | Delete stale Docker/OCI tags (the whole tag, not just `manifest.json`) |
| [`remote-cache`](#remote-cache) | Delete remote-cache artifacts that are download-stale **and** no longer Xray-indexed |

---

## `artifacts`

**What it does:** Runs an AQL query against one Artifactory server and
deletes every matching artifact. The query can be supplied directly
(`--aql`/`--aql-file`), or built for you from `--repo` + `--not-downloaded-since`
using the same "never downloaded, or not downloaded in N months" pattern
used throughout JFrog's documented cleanup recipes — so common cleanups
don't require hand-writing AQL. Always reports the total size of what
matched, before any deletion happens.

**Key flags:**
- `--aql` / `--aql-file` — a literal AQL query, or a path to a file containing one
- `--repo` + `--not-downloaded-since <duration>` — alternative to the above; `<duration>` is an AQL relative time like `3mo`, `6mo`, `1y`, `90d`
- `--csv <path>` — also write matches to CSV
- `--dry-run` (default `true`), no `--yes` gate (matches are only ever what the AQL query said, so there's no separate confirmation beyond re-running with `--dry-run=false`)

**Safety notes:** dry-run (default) issues zero DELETE calls; re-run with `--dry-run=false` to actually delete.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps cleanup artifacts
    participant Art as Artifactory

    User->>CLI: jfps cleanup artifacts --repo R --not-downloaded-since 3mo
    Note over CLI: build AQL query (or use --aql/--aql-file as-is)
    CLI->>Art: POST /artifactory/api/search/aql
    Art-->>CLI: matches (repo, path, name, size, ...)
    Note over CLI: print match table + total size
    opt --csv given
        Note over CLI: write matches to CSV file
    end
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner<br/>no DELETE calls issued
    else --dry-run=false
        loop each match
            CLI->>Art: DELETE /artifactory/{repo}/{path}/{name}
            Art-->>CLI: 204 / error
        end
        Note over CLI: print per-artifact results + delete count
    end
```

---

## `repos`

**What it does:** Bulk-deletes repositories whose keys are listed, one per
line, in a plain text file (blank lines and `#`-comments are skipped).

**Key flags:**
- `--file <path>` (required) — newline-delimited repo keys
- `--dry-run` (default `true`)
- `--yes` — skip the interactive confirmation prompt

**Safety notes:** dry-run skips *all* HTTP calls, not just the deletes. When disabling dry-run, an interactive `y/N` confirmation is also required unless `--yes` is passed — two independent gates before any repository is deleted.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps cleanup repos
    participant Art as Artifactory

    User->>CLI: jfps cleanup repos --file repos.txt --dry-run=false
    Note over CLI: read repo keys from file (local, no HTTP)
    CLI-->>User: print loaded repo list
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner<br/>no HTTP calls issued
    else --dry-run=false
        CLI->>User: confirm "Delete N repo(s)?" [y/N]
        User-->>CLI: y (or --yes was passed, skip prompt)
        loop each repo key
            CLI->>Art: DELETE /artifactory/api/repositories/{repo}
            Art-->>CLI: status code / error
        end
        Note over CLI: print per-repo results + delete count
    end
```

---

## `inactive-users`

**What it does:** Finds Artifactory users who haven't logged in for at least
`--months` months (or never logged in at all), excluding a given realm
(default `internal`, to protect Artifactory's own service accounts), and
optionally deletes them.

**Key flags:**
- `--months` (default `6`) — inactivity threshold
- `--exclude-realm` (default `internal`) — realm to always skip
- `--dry-run` (default `true`)
- `--yes` — skip the interactive confirmation prompt

**Safety notes:** dry-run default; interactive confirmation required to actually delete unless `--yes` is passed.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps cleanup inactive-users
    participant Art as Artifactory

    User->>CLI: jfps cleanup inactive-users --months 6
    CLI->>Art: GET /artifactory/api/security/users
    Art-->>CLI: all users + lastLoggedIn
    Note over CLI: filter: lastLoggedIn==0 OR before cutoff,<br/>excluding --exclude-realm
    CLI-->>User: print candidate table
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner<br/>no DELETE calls issued
    else --dry-run=false
        CLI->>User: confirm "Delete N inactive user(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each candidate
            CLI->>Art: DELETE /access/api/v2/users/{name}
            Art-->>CLI: 204 / error
        end
        Note over CLI: print per-user results + delete count
    end
```

---

## `diff-delete-artifacts`

**What it does:** Compares the same repository across two servers (source
and target) by path+name (or sha256), and deletes from **target only**
whatever exists there but not on source — i.e. brings target back in sync
with source by removing extras (never adds anything).

**Key flags:**
- `--repo` (required) — repo key to compare, same on both servers
- uses `--source-*`/`--target-*` connection flags
- `--dry-run` (default `true`)
- `--yes` — skip the interactive confirmation prompt

**Safety notes:** dry-run default; deletes only ever target the **target** server, never source; interactive confirmation required unless `--yes`.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps cleanup diff-delete-artifacts
    participant Src as Source Artifactory
    participant Tgt as Target Artifactory

    User->>CLI: jfps cleanup diff-delete-artifacts --repo R
    CLI->>Src: POST /artifactory/api/search/aql (list repo R)
    Src-->>CLI: source items
    CLI->>Tgt: POST /artifactory/api/search/aql (list repo R)
    Tgt-->>CLI: target items
    Note over CLI: diff by path/name key:<br/>candidates = target items not in source
    CLI-->>User: print target-only candidate table
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner<br/>no DELETE calls issued
    else --dry-run=false
        CLI->>User: confirm "Delete N target-only artifact(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each candidate
            CLI->>Tgt: DELETE /artifactory/{repo}/{path}/{name}
            Tgt-->>CLI: 204 / error
        end
        Note over CLI: print per-artifact results + delete count
    end
```

---

## `docker-tags`

**What it does:** Finds Docker/OCI tags whose manifest (`manifest.json` or,
for multi-arch images, `list.manifest.json`) has not been downloaded, or
not downloaded in `--older-than`, and deletes the **whole tag directory** —
not just the manifest file. Deleting only the manifest is a well-documented
pitfall: the tag's config blob and layers are left behind as orphaned
storage. Matches on either manifest name are deduplicated to one delete per
tag directory.

**Key flags:**
- `--repo` (required) — Docker/OCI repository to search
- `--older-than <duration>` (required) — e.g. `3mo`, `6mo`, `1y`
- `--csv <path>` — also write matches to CSV
- `--dry-run` (default `true`)
- `--yes` — skip the interactive confirmation prompt

**Safety notes:** dry-run default; interactive confirmation required unless `--yes`; deletes the tag directory with a trailing slash, never the manifest file alone.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps cleanup docker-tags
    participant Art as Artifactory

    User->>CLI: jfps cleanup docker-tags --repo docker-local --older-than 3mo
    CLI->>Art: POST /artifactory/api/search/aql (name=manifest.json, download-stale)
    Art-->>CLI: matches
    CLI->>Art: POST /artifactory/api/search/aql (name=list.manifest.json, download-stale)
    Art-->>CLI: matches
    Note over CLI: dedupe by tag directory (repo/path)
    CLI-->>User: print stale tag table + manifest-size total
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner<br/>no DELETE calls issued
    else --dry-run=false
        CLI->>User: confirm "Delete N Docker tag(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each stale tag
            CLI->>Art: DELETE /artifactory/{repo}/{tag-path}/  (trailing slash = whole tag dir)
            Art-->>CLI: 204 / error
        end
        Note over CLI: print per-tag results + delete count
    end
```

---

## `remote-cache`

**What it does:** Cleans up a remote repository's local cache
(`<remote>-cache`) using two filters in sequence, mirroring the PS "Xray
retention expired" cleanup pattern: first, find artifacts that are
download-stale (never downloaded, or not downloaded in `--older-than`);
then, of those, keep only the ones Xray no longer has an index record for
(never indexed, or the cache entry's Xray retention has expired) — an
artifact Xray still has indexed is left alone even if it's download-stale,
since Xray may still need it. Docker/OCI repos are detected automatically
(or forced via `--package-type`) and cleaned up by tag directory, same as
`docker-tags`.

**Key flags:**
- `--repo` (required) — the **remote** repo key, e.g. `npm-remote` (its `-cache` repo is what's actually searched)
- `--older-than` (default `6mo`)
- `--package-type` (default `auto`) — `auto` \| `docker` \| `other`
- `--batch-size` (default `50`) — paths per Xray Artifact Summary call
- `--csv <path>` — also write remaining candidates to CSV
- `--dry-run` (default `true`)
- `--yes` — skip the interactive confirmation prompt

**Safety notes:** dry-run default; interactive confirmation required unless `--yes`; an artifact still Xray-indexed is never deleted regardless of download staleness.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps cleanup remote-cache
    participant Art as Artifactory
    participant Xray as Xray

    User->>CLI: jfps cleanup remote-cache --repo npm-remote --older-than 6mo
    opt --package-type=auto (default)
        CLI->>Art: GET /artifactory/api/repositories/{repo}
        Art-->>CLI: packageType
    end
    Note over CLI: cacheRepo = {repo}-cache
    CLI->>Art: POST /artifactory/api/search/aql (download-stale, exclude .managed-root paths)
    Art-->>CLI: download-stale candidates
    Note over CLI: print stale-candidate count
    loop each batch of candidates (--batch-size)
        CLI->>Xray: POST /xray/api/v1/summary/artifact {paths:[...]}
        Xray-->>CLI: artifacts[] (still indexed) / errors[] (not found)
        Note over CLI: paths missing from artifacts[] = not indexed / expired
    end
    Note over CLI: expired = download-stale AND not Xray-indexed
    CLI-->>User: print expired-candidate table + total size
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner<br/>no DELETE calls issued
    else --dry-run=false
        CLI->>User: confirm "Delete N artifact(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each expired candidate (deduped by delete target)
            CLI->>Art: DELETE /artifactory/{target}[/]  (trailing slash for Docker tag dirs)
            Art-->>CLI: 204 / error
        end
        Note over CLI: print per-artifact results + delete count
    end
```
