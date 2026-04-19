| name | gh-pm-kit |
| --- | --- |
| description | gh-pm-kit GitHub CLI extension for managing and migrating GitHub Projects (v2 and classic) and Discussions — including cross-host migration, project diff, item listing, and discussion search. |

# gh pm-kit

Project management extensions for the [GitHub CLI](https://cli.github.com/).

## Installation

```sh
gh extension install srz-zumix/gh-pm-kit
```

## Prerequisites

gh CLI must be installed and authenticated before using gh pm-kit.

```sh
# Verify gh CLI is installed
gh --version

# Authenticate with GitHub
gh auth login
```

## CLI Structure

```
gh pm-kit
├── completion              # Shell completion script generation
├── skills                  # AI agent skills management
├── discussions             # GitHub Discussions management
│   ├── list                # List discussions in a repository
│   ├── search [query...]   # Search discussions by query
│   └── migrate             # Migrate discussions to another repository
└── projects                # GitHub Projects management
    ├── list                # List GitHub Projects v2
    ├── item                # Project item management
    │   └── list <number|URL>
    ├── diff <src> <dst>    # Show diff between two projects
    ├── migrate <number|URL> [dst-number|dst-URL]
    └── v1                  # GitHub Projects classic management
        ├── list            # List classic projects
        ├── columns         # Column management
        │   └── list <number|URL>
        ├── cards           # Card management
        │   └── list (<column-id> | <project-url|number> <column-name>)
        └── migrate <number|URL>
```

## Global Flags

The following flags are available on all commands:

| Flag | Default | Description |
| --- | --- | --- |
| `-L, --log-level string` | `info` | Set log level: `debug\|info\|warn\|error` |
| `--read-only` | `false` | Run in read-only mode (prevent write operations) |

## Output Flags

The following flags are available on commands that produce output:

| Flag | Default | Description |
| --- | --- | --- |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

## discussions

### discussions list

List discussions in a repository.

```sh
gh pm-kit discussions list [flags]
```

```sh
# List discussions in the current repository
gh pm-kit discussions list

# List discussions in a specific repository
gh pm-kit discussions list --repo owner/repo

# List discussions on GitHub Enterprise Server
gh pm-kit discussions list --repo HOST/owner/repo

# Output as JSON
gh pm-kit discussions list --format json

# Filter with jq
gh pm-kit discussions list --format json --jq '.[].title'
```

| Flag | Default | Description |
| --- | --- | --- |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `-R, --repo string` | current repo | Repository in the format `[HOST/]OWNER/REPO` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### discussions search

Search discussions in a repository using a search query.

```sh
gh pm-kit discussions search [query...] [flags]
```

```sh
# Search discussions by keyword
gh pm-kit discussions search "bug report"

# Search with label filter
gh pm-kit discussions search --label bug --label enhancement

# Search in a specific repository
gh pm-kit discussions search "feature request" --repo owner/repo

# Search across an owner (org or user)
gh pm-kit discussions search "help" --owner my-org

# Output as JSON
gh pm-kit discussions search "question" --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-R, --repo string` | current repo | Repository in the format `[HOST/]OWNER/REPO` |
| `--owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `-l, --label strings` | | Filter discussions by labels (repeatable) |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### discussions migrate

Migrate discussions from one repository to another (supports cross-host migration).
When `--number` is specified, only that discussion is migrated.
When `--number` is omitted, all discussions are migrated.

Already-migrated discussions are identified by a hidden marker and skipped by default;
use `--overwrite` to re-migrate them.

```sh
gh pm-kit discussions migrate --dst OWNER/REPO [flags]
```

```sh
# Migrate all discussions to another repository
gh pm-kit discussions migrate --dst dest-owner/dest-repo

# Migrate a single discussion by number
gh pm-kit discussions migrate --dst dest-owner/dest-repo --number 42

# Migrate a single discussion by URL
gh pm-kit discussions migrate --dst dest-owner/dest-repo --number https://github.com/owner/repo/discussions/42

# Migrate from a specific source repository
gh pm-kit discussions migrate --repo src-owner/src-repo --dst dest-owner/dest-repo

# Cross-host migration (GitHub Enterprise Server to github.com)
gh pm-kit discussions migrate --repo HOST/owner/repo --dst dest-owner/dest-repo

# Override the destination category
gh pm-kit discussions migrate --dst dest-owner/dest-repo --category general

# Enable Discussions on the destination repository if not already enabled
gh pm-kit discussions migrate --dst dest-owner/dest-repo --enable-discussions

# Overwrite previously migrated discussions
gh pm-kit discussions migrate --dst dest-owner/dest-repo --overwrite

# Migrate without embedding reaction summaries
gh pm-kit discussions migrate --dst dest-owner/dest-repo --no-reactions

# Preview what would be migrated (read-only mode)
gh pm-kit discussions migrate --dst dest-owner/dest-repo --read-only
```

| Flag | Default | Description |
| --- | --- | --- |
| `-R, --repo string` | current repo | Source repository in the format `[HOST/]OWNER/REPO` |
| `-d, --dst string` | **(required)** | Destination repository in the format `[HOST/]OWNER/REPO` |
| `-n, --number string` | | Discussion number or URL to migrate (migrates all if omitted) |
| `--category string` | | Override destination category slug (uses source category slug if omitted) |
| `--enable-discussions` | `false` | Enable Discussions on the destination repository if not already enabled |
| `--overwrite` | `false` | Overwrite a previously migrated discussion identified by its migration marker |
| `--no-reactions` | `false` | Do not embed reaction summaries into migrated discussion and comment bodies |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

## projects

### projects list

List GitHub Projects v2 for an owner.

```sh
gh pm-kit projects list [flags]
```

```sh
# List projects for the current owner
gh pm-kit projects list

# List projects for a specific owner (organization or user)
gh pm-kit projects list --owner my-org

# List projects on GitHub Enterprise Server
gh pm-kit projects list --owner HOST/my-org

# Output as JSON
gh pm-kit projects list --format json

# Get project numbers with jq
gh pm-kit projects list --format json --jq '.[].number'
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### projects item list

List items in a GitHub Project v2.
The project can be specified by its number or by its URL
(e.g. `https://github.com/orgs/my-org/projects/1`).

```sh
gh pm-kit projects item list <number|URL> [flags]
```

```sh
# List items in project #1 for the current owner
gh pm-kit projects item list 1

# List items for a specific owner's project
gh pm-kit projects item list 1 --owner my-org

# List items by project URL
gh pm-kit projects item list https://github.com/orgs/my-org/projects/1

# Show additional built-in fields
gh pm-kit projects item list 1 --field ID,TYPE,NUMBER,TITLE,AUTHOR,URL,ARCHIVED

# Show custom fields
gh pm-kit projects item list 1 --custom-field "Status" --custom-field "Priority"

# Output as JSON
gh pm-kit projects item list 1 --format json

# Get item titles with jq
gh pm-kit projects item list 1 --format json --jq '.[].title'
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--field strings` | `TYPE,NUMBER,TITLE,URL` | Built-in fields to display: `ID\|TYPE\|NUMBER\|TITLE\|AUTHOR\|URL\|ARCHIVED` |
| `--custom-field strings` | | Custom field names to display (any ProjectV2 custom field name) |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### projects diff

Show the differences between a source and destination GitHub Project v2.
Items are matched using migration markers embedded during `projects migrate`,
so this command is most useful after migration.

Diff legend:
- `-` present only in the source (not yet migrated)
- `+` present only in the destination (not matched by a migration marker)
- `~` present in both but with differences (title, archived state, or field values)

Custom fields are compared by name and type (single-select fields also compare option names).

```sh
gh pm-kit projects diff <src-number|src-URL> <dst-number|dst-URL> [flags]
```

```sh
# Diff between projects of different owners
gh pm-kit projects diff 1 2 --src src-owner --dst dst-owner

# Diff using project URLs
gh pm-kit projects diff \
  https://github.com/orgs/src-org/projects/1 \
  https://github.com/orgs/dst-org/projects/2

# Cross-host diff (GitHub Enterprise Server to github.com)
gh pm-kit projects diff \
  https://HOST/orgs/src-org/projects/1 \
  https://github.com/orgs/dst-org/projects/2

# Always show colors
gh pm-kit projects diff 1 2 --src src-owner --dst dst-owner --color always

# Output as JSON
gh pm-kit projects diff 1 2 --src src-owner --dst dst-owner --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-s, --src string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-d, --dst string` | | Destination owner in the format `[HOST/]OWNER` (required unless a destination URL is given) |
| `--color string` | `auto` | Colorize output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### projects migrate

Migrate a GitHub Project v2 from one owner to another.
Copies project metadata, custom fields (TEXT, NUMBER, DATE, SINGLE_SELECT, ITERATION), and items.

Items are migrated as draft issues by default.
If `--repo` is specified, the migration first searches for an existing issue carrying the migration marker
in that repository and links it to the project. If no matching issue is found and `--create-issue` is set,
a new issue is created; otherwise a draft issue is used as a fallback.

If a destination project number or URL is given as the second argument, that project is used as the target.
Without a destination project, a new destination project is created when needed.

Already-migrated items are identified by a hidden marker and skipped by default.

```sh
gh pm-kit projects migrate <number|URL> [dst-number|dst-URL] --dst OWNER [flags]
```

```sh
# Migrate project #1 to another owner
gh pm-kit projects migrate 1 --dst dst-owner

# Migrate by project URL
gh pm-kit projects migrate https://github.com/orgs/my-org/projects/1 --dst dst-owner

# Cross-host migration (GitHub Enterprise Server to github.com)
gh pm-kit projects migrate 1 --src HOST/src-owner --dst dst-owner

# Migrate into an existing destination project
gh pm-kit projects migrate 1 2 --src src-owner --dst dst-owner

# Migrate into an existing destination project by URL
gh pm-kit projects migrate \
  https://HOST/orgs/src-org/projects/1 \
  https://github.com/orgs/dst-org/projects/2

# Migrate items as real issues (search for existing issues by marker)
gh pm-kit projects migrate 1 --dst dst-owner --repo dst-owner/dst-repo

# Migrate items as new issues when no matching issue is found
gh pm-kit projects migrate 1 --dst dst-owner --repo dst-owner/dst-repo --create-issue

# Overwrite previously migrated items
gh pm-kit projects migrate 1 --dst dst-owner --overwrite

# Preview migration without making changes
gh pm-kit projects migrate 1 --dst dst-owner --read-only
```

| Flag | Default | Description |
| --- | --- | --- |
| `-s, --src string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-d, --dst string` | **(required)** | Destination owner in the format `[HOST/]OWNER` (required unless a destination URL is given as the second argument) |
| `-r, --repo string` | | Repository in `[HOST/]OWNER/REPO` format; items are linked to matching issues (by migration marker) in this repository |
| `--create-issue` | `false` | When `--repo` is set and no matching issue is found, create a new issue instead of a draft issue |
| `--overwrite` | `false` | Overwrite previously migrated content identified by the migration marker |

---

## projects v1

### projects v1 list

List GitHub Projects (classic) for an owner or repository.
If `--repo` is specified, repository projects are listed.
If `--owner` is specified, projects for that organization or user are listed.
If neither is specified, the current repository's owner projects are listed.

```sh
gh pm-kit projects v1 list [flags]
```

```sh
# List classic projects for the current owner
gh pm-kit projects v1 list

# List classic projects for a specific owner
gh pm-kit projects v1 list --owner my-org

# List classic projects for a specific repository
gh pm-kit projects v1 list --repo owner/repo

# Output as JSON
gh pm-kit projects v1 list --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-R, --repo string` | | Repository in the format `[HOST/]OWNER/REPO`; lists repository-scoped classic projects |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### projects v1 columns list

List columns of a GitHub Project (classic).
The project can be specified by its number or by its URL
(e.g. `https://github.com/orgs/my-org/projects/1` or `https://github.com/owner/repo/projects/1`).
When a repository-scoped project URL is provided, `--owner` and `--repo` are inferred automatically.

```sh
gh pm-kit projects v1 columns list <number|URL> [flags]
```

```sh
# List columns of classic project #1 for the current owner
gh pm-kit projects v1 columns list 1

# List columns for a specific owner's project
gh pm-kit projects v1 columns list 1 --owner my-org

# List columns by project URL
gh pm-kit projects v1 columns list https://github.com/orgs/my-org/projects/1

# List columns for a repository-scoped project
gh pm-kit projects v1 columns list https://github.com/owner/repo/projects/1

# Output as JSON to get column IDs
gh pm-kit projects v1 columns list 1 --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `-R, --repo string` | | Repository in the format `[HOST/]OWNER/REPO`; for repository-scoped projects |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### projects v1 cards list

List cards in a GitHub Project (classic) column.
Accepts two forms:

- `list <column-id>` — list by numeric column ID (obtained from `projects v1 columns list`)
- `list <project-url|number> <column-name>` — list by project URL (or number) and column name (case-insensitive)

```sh
gh pm-kit projects v1 cards list <column-id> [flags]
gh pm-kit projects v1 cards list <project-url|number> <column-name> [flags]
```

```sh
# List cards by column ID
gh pm-kit projects v1 cards list 12345678

# List cards by project number and column name
gh pm-kit projects v1 cards list 1 "In Progress"

# List cards by project URL and column name (case-insensitive)
gh pm-kit projects v1 cards list https://github.com/orgs/my-org/projects/1 "To Do"

# List cards for a repository-scoped project
gh pm-kit projects v1 cards list https://github.com/owner/repo/projects/1 "Done"

# Output as JSON
gh pm-kit projects v1 cards list 1 "Backlog" --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER`; used with the two-argument form |
| `-R, --repo string` | | Repository in the format `[HOST/]OWNER/REPO`; for repository-scoped projects |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

---

### projects v1 migrate

Migrate a GitHub Project (classic) to a new GitHub Projects v2 project.
A new Projects v2 project is created under the destination owner.
Each column becomes an option in a `Column` single-select field,
and each card is migrated as a draft issue with the `Column` field set.
Already-migrated items are identified by a hidden marker and skipped unless `--overwrite` is specified.

```sh
gh pm-kit projects v1 migrate <number|URL> --dst OWNER [flags]
```

```sh
# Migrate classic project #1 to a v2 project under another owner
gh pm-kit projects v1 migrate 1 --dst dst-owner

# Migrate by project URL (org-level)
gh pm-kit projects v1 migrate https://github.com/orgs/my-org/projects/1 --dst dst-owner

# Migrate by repository-scoped project URL
gh pm-kit projects v1 migrate https://github.com/owner/repo/projects/1 --dst dst-owner

# Migrate for a specific source owner
gh pm-kit projects v1 migrate 1 --owner src-owner --dst dst-owner

# Migrate a repository-scoped classic project
gh pm-kit projects v1 migrate 1 --repo src-owner/src-repo --dst dst-owner

# Overwrite previously migrated items
gh pm-kit projects v1 migrate 1 --dst dst-owner --overwrite

# Preview migration without making changes
gh pm-kit projects v1 migrate 1 --dst dst-owner --read-only
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Source owner in the format `[HOST/]OWNER`; inferred from URL if a project URL is given |
| `-R, --repo string` | | Source repository in the format `[HOST/]OWNER/REPO`; for repository-scoped classic projects; inferred from URL if a repo-scoped project URL is given |
| `-d, --dst string` | **(required)** | Destination owner in the format `[HOST/]OWNER` |
| `--overwrite` | `false` | Re-migrate already-migrated items instead of skipping them |

---

## Common Workflows

### Migrate all GitHub Projects v2 items to another organization

```sh
# Step 1: List source projects to find project numbers
gh pm-kit projects list --owner src-org

# Step 2: Migrate a project
gh pm-kit projects migrate 1 --src src-org --dst dst-org

# Step 3: Verify migration with diff
gh pm-kit projects diff \
  https://github.com/orgs/src-org/projects/1 \
  https://github.com/orgs/dst-org/projects/1
```

### Cross-host migration (GitHub Enterprise Server → github.com)

```sh
# Migrate a project from GHES to github.com
gh pm-kit projects migrate 1 \
  --src ghes.example.com/src-org \
  --dst dst-org

# Migrate all discussions from GHES to github.com
gh pm-kit discussions migrate \
  --repo ghes.example.com/src-owner/src-repo \
  --dst dst-owner/dst-repo
```

### Migrate classic projects (v1) to Projects v2

```sh
# Step 1: List classic projects
gh pm-kit projects v1 list --owner my-org

# Step 2: Preview columns before migration
gh pm-kit projects v1 columns list 1 --owner my-org

# Step 3: Migrate classic project to v2
gh pm-kit projects v1 migrate 1 --owner my-org --dst my-org

# Step 4: List migrated items in the new v2 project
gh pm-kit projects list --owner my-org --format json \
  --jq '.[] | select(.title == "My Project") | .number'
```

### Migrate discussions with reaction summaries disabled

```sh
# Migrate all discussions without embedding reaction counts
gh pm-kit discussions migrate \
  --repo src-owner/src-repo \
  --dst dst-owner/dst-repo \
  --no-reactions
```

### Idempotent re-migration

```sh
# Re-run migration overwriting previously migrated items
gh pm-kit projects migrate 1 --dst dst-owner --overwrite

# Re-run discussion migration overwriting previously migrated discussions
gh pm-kit discussions migrate --dst dst-owner/dst-repo --overwrite
```

### Migrate project items as real issues

```sh
# Items are linked to existing issues (by migration marker) in the target repo,
# or new issues are created when none is found
gh pm-kit projects migrate 1 \
  --dst dst-owner \
  --repo dst-owner/dst-repo \
  --create-issue
```

### Inspect items in a project

```sh
# List all items with default fields
gh pm-kit projects item list 1 --owner my-org

# List all items including custom fields
gh pm-kit projects item list 1 --owner my-org \
  --custom-field "Status" --custom-field "Priority"

# Export items to JSON
gh pm-kit projects item list 1 --owner my-org --format json > items.json
```

### Search for discussions by label

```sh
# Search discussions tagged with a specific label
gh pm-kit discussions search --label bug --repo owner/repo

# Search across an owner with multiple labels
gh pm-kit discussions search --label bug --label "needs-triage" --owner my-org

# Combine keyword and label filter
gh pm-kit discussions search "login error" --label bug --repo owner/repo
```

---

## Migration Markers

gh pm-kit embeds hidden HTML comments into migrated items to enable idempotent re-runs:

- **Project marker** — embedded in the project readme to identify the migration source
- **Item marker** — embedded in draft issue / issue bodies to identify the original item

These markers are SHA-256 hashed from the source host, owner, and project number to prevent
information leakage and cross-host collisions. They are not visible in the rendered UI.

When a marker is found in the destination:
- The item / project is **skipped** by default
- Use `--overwrite` to delete and recreate it
- Use `--prune-items` (destructive, hidden flag) to delete **all** marked items before migrating

---

## Output Formatting

### JSON Output

```sh
# List all projects as JSON
gh pm-kit projects list --format json

# Extract project titles with jq
gh pm-kit projects list --format json --jq '.[].title'

# Get all open discussion titles
gh pm-kit discussions list --format json --jq '.[].title'

# Get column IDs from a classic project
gh pm-kit projects v1 columns list 1 --format json --jq '.[] | {id: .id, name: .name}'
```

### Go Template Output

```sh
# Custom template for project list
gh pm-kit projects list --format json \
  --template '{{range .}}{{.number}}: {{.title}}{{"\n"}}{{end}}'
```

---

## References

- Repository: <https://github.com/srz-zumix/gh-pm-kit>
- GitHub CLI: <https://cli.github.com/>
- GitHub Projects v2 docs: <https://docs.github.com/en/issues/planning-and-tracking-with-projects>
- GitHub Discussions docs: <https://docs.github.com/en/discussions>
