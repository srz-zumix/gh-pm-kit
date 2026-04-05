# gh pm-kit

Project management extensions for the [GitHub CLI](https://cli.github.com/).

## Installation

```sh
gh extension install srz-zumix/gh pm-kit
```

### Shell completion

`gh pm-kit` supports shell completion via the `completion` subcommand.
Run `gh pm-kit completion --help` for details on how to configure it for your shell.

## Usage

```sh
gh pm-kit [command] [flags]
```

Global flags available on all commands:

| Flag | Default | Description |
| --- | --- | --- |
| `-L, --log-level string` | `info` | Set log level: `debug\|info\|warn\|error` |
| `--read-only` | `false` | Run in read-only mode (prevent write operations) |

---

## discussions

### discussions list

List discussions in a repository.

```sh
gh pm-kit discussions list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `-R, --repo string` | current repo | Repository in the format `[HOST/]OWNER/REPO` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### discussions migrate

Migrate discussions from one repository to another (supports cross-host migration).
When `--number` is specified, only that discussion is migrated.
When `--number` is omitted, all discussions are migrated.

```sh
gh pm-kit discussions migrate --dst OWNER/REPO [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-R, --repo string` | current repo | Source repository in the format `[HOST/]OWNER/REPO` |
| `-d, --dst string` | **(required)** | Destination repository in the format `[HOST/]OWNER/REPO` |
| `-n, --number string` | | Discussion number or URL to migrate (migrates all if omitted) |
| `--category string` | | Override destination category slug (uses source category slug if omitted) |
| `--enable-discussions` | `false` | Enable Discussions on the destination repository if not already enabled |
| `--overwrite` | `false` | Overwrite a previously migrated discussion identified by its migration marker; without this flag, marked discussions are skipped and unmarked discussions get a new copy created |
| `--no-reactions` | `false` | Do not embed reaction summaries into migrated discussion and comment bodies |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### discussions search

Search discussions in a repository using a search query.
The query can include label filters and other search criteria.

```sh
gh pm-kit discussions search [query...] [flags]
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

## projects

### projects list

List GitHub Projects v2 for an owner.

```sh
gh pm-kit projects list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects item list

List items in a GitHub Project v2.
The project can be specified by its number or by its URL (e.g. `https://github.com/orgs/my-org/projects/1`).

```sh
gh pm-kit projects item list <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--field strings` | `TYPE,NUMBER,TITLE,URL` | Built-in fields to display: `ID\|TYPE\|NUMBER\|TITLE\|AUTHOR\|URL\|ARCHIVED` |
| `--custom-field strings` | | Custom field names to display (any ProjectV2 custom field name) |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects migrate

Migrate a GitHub Project v2 (New Projects) from one owner to another.
Copies project metadata, custom fields (TEXT, NUMBER, DATE, SINGLE_SELECT, ITERATION), and items.

Items are migrated as draft issues by default.
If `--repo` is specified, the migration first searches for an existing issue carrying the migration marker in that repository and links it to the project.
If no matching issue is found and `--create-issue` is set, a new issue is created; otherwise a draft issue is used as a fallback.

If a destination project number or URL is given as the second argument, that project is used as the migration target.
Without a destination project, a new destination project is created when needed.

Items already migrated are identified by a hidden marker and skipped by default.

```sh
gh pm-kit projects migrate <number|URL> [dst-number|dst-URL] --dst OWNER [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-s, --src string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-d, --dst string` | **(required)** | Destination owner in the format `[HOST/]OWNER` (required unless a destination URL is given as the second argument) |
| `-r, --repo string` | | Repository in `[HOST/]OWNER/REPO` format; items are linked to matching issues (by migration marker) in this repository |
| `--create-issue` | `false` | When `--repo` is set and no matching issue is found, create a new issue instead of a draft issue |
| `--overwrite` | `false` | Overwrite previously migrated content identified by the migration marker: when no destination project is given, overwrite the existing migrated project instead of skipping it; for migrated items, delete and re-create them instead of skipping them |

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

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-R, --repo string` | | Repository in the format `[HOST/]OWNER/REPO`; lists repository-scoped classic projects |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects v1 columns list

List columns of a GitHub Project (classic).
The project can be specified by its number or by its URL
(e.g. `https://github.com/orgs/my-org/projects/1` or `https://github.com/owner/repo/projects/1`).
When a repository-scoped project URL is provided, the `--owner` and `--repo` flags are inferred automatically.

```sh
gh pm-kit projects v1 columns list <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `-R, --repo string` | | Repository in the format `[HOST/]OWNER/REPO`; for repository-scoped projects |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects v1 cards list

List cards in a GitHub Project (classic) column.
Accepts two forms:

- `list <column-id>` — list by numeric column ID (obtained from `projects v1 columns list`)
- `list <project-url|number> <column-name>` — list by project URL (or number) and column name (case-insensitive)

When a repository-scoped project URL is provided, the `--owner` and `--repo` flags are inferred automatically.

```sh
gh pm-kit projects v1 cards list <column-id> [flags]
gh pm-kit projects v1 cards list <project-url|number> <column-name> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER`; used with the two-argument form |
| `-R, --repo string` | | Repository in the format `[HOST/]OWNER/REPO`; for repository-scoped projects |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects v1 migrate

Migrate a GitHub Project (classic) to a new GitHub Projects v2 project.
The source classic project is specified by its number or URL.
Both org-level (`https://github.com/orgs/my-org/projects/1`) and repository-scoped
(`https://github.com/owner/repo/projects/1`) project URLs are supported.

A new Projects v2 project is created under the destination owner.
Each column becomes an option in a `Column` single-select field,
and each card is migrated as a draft issue with the `Column` field set.
Already-migrated items are identified by a hidden marker and skipped unless `--overwrite` is specified.

```sh
gh pm-kit projects v1 migrate <number|URL> --dst OWNER [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Source owner in the format `[HOST/]OWNER`; inferred from URL if a project URL is given |
| `-R, --repo string` | | Source repository in the format `[HOST/]OWNER/REPO`; for repository-scoped classic projects; inferred from URL if a repo-scoped project URL is given |
| `-d, --dst string` | **(required)** | Destination owner in the format `[HOST/]OWNER` |
| `--overwrite` | `false` | Re-migrate already-migrated items instead of skipping them |
