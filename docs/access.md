# `jfps access`

Manages Artifactory/Access-layer identity and authorization objects:
permission targets (via both the legacy Security API and Access API v2),
groups, users, and access tokens.

**Safety model:** read-only subcommands (`list-*`, `show-*`, `find-empty-*`,
`token-expiry`, `audit`, `backup`, `effective-permissions`) only ever issue
GET calls. Mutating subcommands (`create-permission`, `grant-path`,
`add-repos`, `remove-repos`, `restore`, `delete-users`, `delete-groups`,
`delete-permissions`) default `--dry-run` to `true`. `restore` and the three
`delete-*` commands additionally require an interactive `y/N` confirmation
once `--dry-run=false` is passed, unless `--yes` is given; the three
`delete-*` commands also always exclude a hardcoded list of protected names
(e.g. `admin`, `anonymous`) plus anything in `--exclude`. `restore` is
additionally always additive-only: an entity that already exists on the
target is skipped, never updated or overwritten.

| Subcommand | What it does |
|---|---|
| [`list-permissions`](#list-permissions) | List all permission target names |
| [`show-permission`](#show-permission) | Show one permission target's full detail |
| [`find-empty-permissions`](#find-empty-permissions) | Find permission targets with zero repos assigned |
| [`list-users-in-permission`](#list-users-in-permission) | List usernames assigned to a permission target |
| [`find-empty-groups`](#find-empty-groups) | Find groups with no members |
| [`token-expiry`](#token-expiry) | Report access tokens expiring within N days |
| [`audit`](#audit) | Find permission targets whose repo targets match a pattern |
| [`create-permission`](#create-permission) | Create a new v2 permission target |
| [`grant-path`](#grant-path) | Create-or-merge repo/path/principal grants on a permission target |
| [`add-repos`](#add-repos) | Add repo keys to a permission target |
| [`remove-repos`](#remove-repos) | Remove repo keys from a permission target |
| [`backup`](#backup) | Export users/groups/permissions to local JSON files |
| [`restore`](#restore) | Recreate users/groups/permissions from a backup directory on another JPD |
| [`delete-users`](#delete-users) | Bulk-delete users listed in a file |
| [`delete-groups`](#delete-groups) | Bulk-delete groups listed in a file |
| [`delete-permissions`](#delete-permissions) | Bulk-delete permission targets listed in a file |
| [`effective-permissions`](#effective-permissions) | Show who can access a repo, and at what action level |

---

## `list-permissions`

**What it does:** Lists all permission target names/URIs.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access list-permissions
    participant JPD

    User->>CLI: jfps access list-permissions
    CLI->>JPD: GET /artifactory/api/security/permissions
    JPD-->>CLI: permission target names
    CLI-->>User: print table
```

## `show-permission`

**What it does:** Shows one permission target's full detail (assigned repos + principals).

**Key flags:** `--name` (required)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access show-permission
    participant JPD

    User->>CLI: jfps access show-permission --name P
    CLI->>JPD: GET /artifactory/api/security/permissions/P
    JPD-->>CLI: full permission target detail
    CLI-->>User: print detail
```

## `find-empty-permissions`

**What it does:** Finds permission targets with zero repositories assigned (excluding the built-in `INTERNAL` target).

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access find-empty-permissions
    participant JPD

    User->>CLI: jfps access find-empty-permissions
    CLI->>JPD: GET /artifactory/api/security/permissions
    JPD-->>CLI: all permission target names
    loop each target (except INTERNAL)
        CLI->>JPD: GET /artifactory/api/security/permissions/{name}
        JPD-->>CLI: detail
        Note over CLI: keep if repo count == 0
    end
    CLI-->>User: print empty-target table
```

## `list-users-in-permission`

**What it does:** Lists usernames assigned to a permission target.

**Key flags:** `--name` (required)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access list-users-in-permission
    participant JPD

    User->>CLI: jfps access list-users-in-permission --name P
    CLI->>JPD: GET /artifactory/api/security/permissions/P
    JPD-->>CLI: principals.users
    CLI-->>User: print user list
```

## `find-empty-groups`

**What it does:** Finds groups with no members.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access find-empty-groups
    participant JPD

    User->>CLI: jfps access find-empty-groups
    CLI->>JPD: GET /artifactory/api/security/groups
    JPD-->>CLI: all group names
    loop each group
        CLI->>JPD: GET /artifactory/api/security/groups/{name}?includeUsers=true
        JPD-->>CLI: members
        Note over CLI: keep if member count == 0
    end
    CLI-->>User: print empty-group table
```

## `token-expiry`

**What it does:** Reports access tokens expiring within `--days` (or already expired). Non-expiring tokens (`expiry==0`) are always skipped.

**Key flags:** `--days` (default `30`)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access token-expiry
    participant JPD

    User->>CLI: jfps access token-expiry --days 30
    CLI->>JPD: GET /access/api/v1/tokens
    JPD-->>CLI: all tokens + expiry
    Note over CLI: filter: 0 < expiry < now+days
    CLI-->>User: print expiring-token table
```

## `audit`

**What it does:** Finds v2 permission targets whose repo targets match a regex pattern, or a wildcard (`ANY`, `ANY LOCAL`, `ANY REMOTE`) scoped by `--repo-type`.

**Key flags:** `--repo-pattern` (required, regex), `--repo-type` (optional)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access audit
    participant JPD

    User->>CLI: jfps access audit --repo-pattern "^prod-.*"
    CLI->>JPD: GET /access/api/v2/permissions
    JPD-->>CLI: permission names
    loop each permission
        CLI->>JPD: GET /access/api/v2/permissions/{name}
        JPD-->>CLI: resources.artifact.targets
        Note over CLI: match target keys against pattern/wildcard
    end
    CLI-->>User: print matching-permission table
```

## `create-permission`

**What it does:** Creates a brand-new v2 permission target scoped to a set of repos, with no principals yet. Fails loudly if the name already exists (no dry-run — a create either succeeds or fails).

**Key flags:** `--name` (required), `--repos` (required, comma-separated)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access create-permission
    participant JPD

    User->>CLI: jfps access create-permission --name P --repos r1,r2
    CLI->>JPD: POST /access/api/v2/permissions
    JPD-->>CLI: 201 Created / 409 Conflict
    CLI-->>User: print result
```

## `grant-path`

**What it does:** Idempotently creates a v2 permission target, or merges new repo/path/principal grants into an existing one.

**Key flags:** `--permission` (required), `--repos` (required), `--path` (required, `;`-separated include patterns), `--groups`/`--users` (optional `name:ACT+ACT` lists), `--resource` (default `artifact`), `--dry-run` (default `true`)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access grant-path
    participant JPD

    User->>CLI: jfps access grant-path --permission P --repos r1 --path "**"
    CLI->>JPD: GET /access/api/v2/permissions/P
    JPD-->>CLI: existing detail, or 404
    alt dry-run (default)
        Note over CLI: print the would-be plan, stop here
    else --dry-run=false, permission not found
        CLI->>JPD: POST /access/api/v2/permissions
        JPD-->>CLI: created
    else --dry-run=false, permission found
        CLI->>JPD: PATCH /access/api/v2/permissions/P/{resource}
        JPD-->>CLI: merged
    end
```

## `add-repos`

**What it does:** Adds repository keys to a permission target's `artifact` resource targets (no-op for repos already present).

**Key flags:** `--permission` (required), `--repos` (required), `--dry-run` (default `true`)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access add-repos
    participant JPD

    User->>CLI: jfps access add-repos --permission P --repos r3
    CLI->>JPD: GET /access/api/v2/permissions/P
    JPD-->>CLI: current targets
    Note over CLI: compute before/after target set
    alt dry-run (default)
        Note over CLI: print before/after, stop here
    else --dry-run=false
        CLI->>JPD: PUT /access/api/v2/permissions/P
        JPD-->>CLI: updated
    end
```

## `remove-repos`

**What it does:** Removes repository keys from a permission target's `artifact` resource targets, by exact key match (fixes a substring-match bug present in the legacy bash equivalent).

**Key flags:** `--permission` (required), `--repos` (required), `--dry-run` (default `true`)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access remove-repos
    participant JPD

    User->>CLI: jfps access remove-repos --permission P --repos r3
    CLI->>JPD: GET /access/api/v2/permissions/P
    JPD-->>CLI: current targets
    Note over CLI: compute targets with r3 removed (exact match)
    alt dry-run (default)
        Note over CLI: print before/after, stop here
    else --dry-run=false
        CLI->>JPD: PUT /access/api/v2/permissions/P
        JPD-->>CLI: updated
    end
```

## `backup`

**What it does:** Exports users, groups, and/or permissions to local JSON files. Read-only against the server — only local files are written. `users.json`/`groups.json` each hold a JSON array of full per-user/per-group detail (not just names), and `permissions.json` a JSON array of full permission targets — this is the same file layout `restore` reads back in.

**Key flags:** `--output` (required, output directory), `--users-only`/`--groups-only`/`--permissions-only` (default: all three)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access backup
    participant JPD

    User->>CLI: jfps access backup --output ./backup
    opt users enabled
        CLI->>JPD: GET /access/api/v2/users
        JPD-->>CLI: usernames
        loop each username
            CLI->>JPD: GET /access/api/v2/users/{username}
            JPD-->>CLI: full detail
        end
        Note over CLI: write users.json
    end
    opt groups enabled
        CLI->>JPD: GET /access/api/v2/groups
        JPD-->>CLI: group names
        loop each group
            CLI->>JPD: GET /access/api/v2/groups/{name}
            JPD-->>CLI: full detail
        end
        Note over CLI: write groups.json
    end
    opt permissions enabled
        CLI->>JPD: GET /access/api/v2/permissions
        JPD-->>CLI: permission names
        loop each permission
            CLI->>JPD: GET /access/api/v2/permissions/{name}
            JPD-->>CLI: detail
        end
        Note over CLI: write permissions.json
    end
```

## `restore`

**What it does:** Reads a backup directory produced by `backup` and recreates on the target server whatever users, groups, and permissions from it don't already exist there (in that order, since groups reference users and permissions reference both). An entity that already exists on the target is always **skipped**, never updated or overwritten — this is a purely additive operation, useful for cloning identity/access setup onto a new JPD or merging one org's users/groups into another's.

Since a backup never carries a raw password (the Access API doesn't return one), every recreated user has `internal_password_disabled` forced to `true` regardless of its original realm — they'll need to authenticate via SSO or reset their password out of band.

**Key flags:** `--input` (required, a `backup` output directory), `--user-suffix`/`--group-suffix` (appended to every restored name, and to every user/group reference inside restored group memberships and permission principals — handy for merging two orgs' identities without name collisions), `--include-users`/`--include-groups`/`--include-permissions` (comma-separated original names; default: all), `--dry-run` (default `true`), `--yes`

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access restore
    participant JPD

    User->>CLI: jfps access restore --input ./backup --dry-run=false
    Note over CLI: read users.json/groups.json/permissions.json (missing file = nothing of that kind)
    loop each user, then each group, then each permission
        CLI->>JPD: GET .../{name} (does it already exist?)
        JPD-->>CLI: 200 (skip) or 404 (candidate to create)
    end
    alt dry-run (default)
        Note over CLI: print the create/skip plan, stop here
    else --dry-run=false
        CLI->>User: confirm "Create N entit(ies)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each candidate, in order: users, groups, permissions
            CLI->>JPD: POST /access/api/v2/{users|groups|permissions}
            JPD-->>CLI: 201 Created / error
        end
    end
```

## `delete-users`

**What it does:** Bulk-deletes users listed (one per line) in a file, minus always-protected names (`admin`, `anonymous`) and anything in `--exclude`.

**Key flags:** `--file` (required), `--exclude` (comma-separated), `--dry-run` (default `true`), `--yes`

**Safety notes:** dry-run issues **zero** network calls at all (the candidate list is entirely local); when disabled, an interactive confirmation is also required unless `--yes` is passed.

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access delete-users
    participant JPD

    User->>CLI: jfps access delete-users --file users.txt --dry-run=false
    Note over CLI: read file, drop protected/excluded names (local only)
    alt dry-run (default)
        Note over CLI: print [DRY RUN] banner, no network calls at all
    else --dry-run=false
        CLI->>User: confirm "Delete N user(s)?" [y/N]
        User-->>CLI: y (or --yes was passed)
        loop each remaining name
            CLI->>JPD: DELETE /access/api/v2/users/{name}
            JPD-->>CLI: 204 / error
        end
    end
```

## `delete-groups`

**What it does:** Bulk-deletes groups listed in a file, minus always-protected names (`readers`, `Anyremote`, `admin`) and anything in `--exclude`. Same flow/flags/safety gates as `delete-users`, against `DELETE /access/api/v2/groups/{name}`.

## `delete-permissions`

**What it does:** Bulk-deletes permission targets listed in a file, minus always-protected names (`Any Remote`, `Anything`, `INTERNAL`) and anything in `--exclude`. Same flow/flags/safety gates as `delete-users`, against `DELETE /access/api/v2/permissions/{name}`.

## `effective-permissions`

**What it does:** For one or more repos, shows every user and group with any access to it and at what action level (`delete`/`deploy`/`annotate`/`read`/`distribute`/`managedXrayMeta`), plus which permission target(s) grant it. This is the inverse question from `audit` (which starts from a permission target and asks "which repos does it cover"); this instead starts from a repo and asks "who can touch it and how" — useful for answering an access-review question about one specific repository without having to manually cross-reference every permission target.

**Key flags:** `--repos` (required, comma-separated repo keys)

```mermaid
sequenceDiagram
    actor User
    participant CLI as jfps access effective-permissions
    participant JPD

    User->>CLI: jfps access effective-permissions --repos repo1,repo2
    loop each repo
        CLI->>JPD: GET /artifactory/api/artifactpermissions?repoKey={repo}
        JPD-->>CLI: userEffectivePermissions + groupEffectivePermissions
    end
    CLI-->>User: print one row per user/group per repo
```
