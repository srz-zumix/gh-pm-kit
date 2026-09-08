# gh pm-kit

Project management extensions for the [GitHub CLI](https://cli.github.com/).

## Installation

```sh
gh extension install srz-zumix/gh-pm-kit
```

## Shell Completion

**Workaround Available!** While gh CLI doesn't natively support extension completion, we provide a patch script that enables it.

**Prerequisites:** Before setting up gh-pm-kit completion, ensure gh CLI completion is configured for your shell. See [gh completion documentation](https://cli.github.com/manual/gh_completion) for setup instructions.

For detailed installation instructions and setup for each shell, see the [Shell Completion Guide](https://github.com/srz-zumix/go-gh-extension/blob/main/docs/shell-completion.md).

## Agent Skills

gh-pm-kit bundles agent skills for AI. Use the `skills` subcommand to install and manage them.

```sh
gh pm-kit skills [subcommand] [args...]
```

For details, see [Songmu/skillsmith](https://github.com/Songmu/skillsmith).

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
| `--interval duration` | `1s` | Wait time before each content-creating request (discussion, comment, reply) to avoid GitHub's secondary rate limit; set to `0` to disable |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### discussions repos

List repositories owned by an owner that have Discussions enabled.

```sh
gh pm-kit discussions repos [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--all` | `false` | List all repositories, including those without Discussions enabled |
| `--color string` | `auto` | Use color in output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |
| `--owner string` | current owner | Owner in the format `[HOST/]OWNER` |

### discussions search

Search discussions in a repository using a search query.
When `--owner` is set without `--repo`, discussions are searched across all repositories owned by the owner instead.
The query can include label filters and other search criteria.

```sh
gh pm-kit discussions search [query...] [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-R, --repo string` | current repo | Repository in the format `[HOST/]OWNER/REPO` |
| `--owner string` | current owner | Owner in the format `[HOST/]OWNER`; searches across all repositories owned by the owner when `--repo` is not set |
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

### projects field list

List the field definitions of a GitHub Project v2, including built-in fields.
By default each field is shown on a single row with its select options or iteration count summarized.
Use `--show-options` to expand every select option and iteration (including completed ones) onto its own row.
The project can be specified by its number or by its URL (e.g. `https://github.com/orgs/my-org/projects/1`).

```sh
gh pm-kit projects field list <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--show-options` | `false` | Expand select options and iterations onto individual rows |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects item list

List items in a GitHub Project v2.
The project can be specified by its number or by its URL (e.g. `https://github.com/orgs/my-org/projects/1`).
Archived items are included when the host supports them (GitHub.com and recent GitHub Enterprise Server); older hosts return only non-archived items.

```sh
gh pm-kit projects item list <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--field strings` | `TYPE,NUMBER,TITLE,URL` | Built-in fields to display: `ID\|TYPE\|NUMBER\|TITLE\|AUTHOR\|URL\|ARCHIVED\|STATE\|REPOSITORY\|ASSIGNEES\|LABELS\|MILESTONE\|CREATED_AT\|UPDATED_AT\|CLOSED_AT` |
| `--custom-field strings` | | Custom field names to display (any ProjectV2 custom field name) |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects status list

List the status updates posted on a GitHub Project v2, newest first.
Each update shows its posted date, status (`INACTIVE`, `ON_TRACK`, `AT_RISK`, `OFF_TRACK`, `COMPLETE`), start and target dates, creator, and the first line of its body.
Use `--format json` to retrieve full bodies.
The project can be specified by its number or by its URL (e.g. `https://github.com/orgs/my-org/projects/1`).

```sh
gh pm-kit projects status list <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects view list

List the views configured in a GitHub Project v2, including their layout, filter, grouping and sort criteria.
Useful for auditing how a project is presented to its users.
The project can be specified by its number or by its URL (e.g. `https://github.com/orgs/my-org/projects/1`).

```sh
gh pm-kit projects view list <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--field strings` | `NUMBER,NAME,LAYOUT,FILTER,GROUPBY,SORTBY` | Fields to display: `ID\|NUMBER\|NAME\|LAYOUT\|FILTER\|GROUPBY\|VERTICALGROUPBY\|SORTBY\|VISIBLEFIELDS` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects diff

Show the differences between a source and destination GitHub Project v2.
Items are matched using the migration markers embedded during `projects migrate`, so this command is most useful after migration.

Custom fields are compared by name and type (single-select fields also compare option names).
Items are shown as:

- `-` present only in the source (not yet migrated)
- `+` present only in the destination (not matched by a migration marker)
- `~` present in both but with differences (title, archived state, or field values)

```sh
gh pm-kit projects diff <src-number|src-URL> <dst-number|dst-URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-s, --src string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-d, --dst string` | | Destination owner in the format `[HOST/]OWNER` (required unless a destination URL is given) |
| `--color string` | `auto` | Colorize output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

### projects lint

Check a GitHub Project v2 against operational rules and report every violation.
Findings whose severity is at or above `--fail-on` make the command exit
non-zero, which makes the command suitable for scheduled CI runs.

```sh
gh pm-kit projects lint <number|URL> [flags]
```

| Rule | Name | Default severity | Description |
| --- | --- | --- | --- |
| `PM001` | `no-status` | warning | Item has no value in the status field |
| `PM002` | `missing-required-field` | warning | Item has no value in a field listed in `--require` |
| `PM003` | `draft-issue-remaining` | info | Item is still a draft issue and is not tracked in a repository |
| `PM004` | `stale-status-update` | warning | No status update has been posted within `--status-update-days` |
| `PM005` | `unused-select-option` | info | A select option is not used by any item |
| `PM006` | `broken-view-filter` | info | A view filter references an unknown qualifier or field |
| `PM007` | `duplicate-draft` | warning | Multiple draft issues share the same title |
| `PM010` | `closed-issue-not-done` | error | Linked issue or pull request is closed but the status is not a done status |
| `PM011` | `done-but-open` | error | Status is a done status but the linked issue or pull request is still open |
| `PM012` | `stale-item` | warning | Unfinished item has not been updated within `--stale-days` |
| `PM013` | `unassigned-in-progress` | warning | Item is in an in-progress status but has no assignee |
| `PM014` | `archived-but-open` | warning | Item is archived but the linked issue or pull request is still open |
| `PM015` | `past-iteration-incomplete` | warning | Item is still assigned to a completed iteration but is not finished |
| `PM016` | `orphaned-item` | error | Linked issue or pull request is no longer accessible |

`PM006` compares filter qualifiers with the project field names, so views that
use qualifiers this version does not know about are reported as findings.
Archived items are only checked when `--include-archived` is given, except for
`PM014` which always inspects archived items.

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--config string` | `.github/pm-kit.yml`, `.github/pm-kit.yaml` | Configuration file to read lint defaults from |
| `--rule strings` | all rules | Rule IDs to run |
| `--ignore strings` | | Rule IDs to skip |
| `--status-field string` | `Status` | Name of the single-select field that holds the item status |
| `--done-status strings` | `Done,Closed,Complete,Completed` | Status values that mean the work is finished |
| `--in-progress-status strings` | `In Progress,In Review,Doing` | Status values that mean the work is ongoing |
| `--require strings` | | Field names that every item must have a value for (`PM002`) |
| `--stale-days int` | `30` | Days after which an unfinished item is reported as stale (`PM012`) |
| `--status-update-days int` | `14` | Days after which the latest status update is reported as stale (`PM004`) |
| `--include-archived` | `false` | Check archived items as well |
| `--fail-on string` | `error` | Lowest severity that makes the command exit non-zero: `error\|warning\|info` |
| `--exit-zero` | `false` | Always exit with code 0, even when findings are reported |
| `--annotate` | `false` | Report every finding as a GitHub Actions workflow annotation |
| `--summary-markdown string` | | Append a Markdown report to the given file (`-` for stdout), e.g. `"$GITHUB_STEP_SUMMARY"` |
| `--color string` | `auto` | Colorize output: `always\|never\|auto` |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

#### Configuration file

Lint defaults can be stored in `.github/pm-kit.yml` (or `.github/pm-kit.yaml`), which is loaded automatically from the current directory.
Every key is optional, unknown keys are rejected, and flags always take precedence over the file.
The `severity` map is the only way to change the severity of a rule.

```yaml
lint:
  status-field: Status
  done-statuses: [Done, Shipped]
  in-progress-statuses: [In Progress, In Review]
  required-fields: [Priority]
  stale-days: 30
  status-update-days: 14
  include-archived: false
  fail-on: error
  rules: []          # empty means every rule
  ignore: [PM003, PM005]
  severity:
    PM012: error
```

#### GitHub Actions integration

`--annotate` turns every finding into a workflow annotation and `--summary-markdown` appends a
Markdown report to the job summary, so a scheduled workflow can keep a project healthy.
Annotations go to stdout, or to stderr when `--format` is used, so the exported data stays parsable.

```yaml
name: project-lint
on:
  schedule:
    - cron: "0 0 * * 1"
  workflow_dispatch:

permissions: {}

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - run: gh extension install srz-zumix/gh-pm-kit
        env:
          GH_TOKEN: ${{ secrets.PROJECT_TOKEN }}
      - run: |
          gh pm-kit projects lint 1 --owner my-org \
            --annotate \
            --summary-markdown "$GITHUB_STEP_SUMMARY" \
            --fail-on warning
        env:
          GH_TOKEN: ${{ secrets.PROJECT_TOKEN }}
```

The token needs the `read:project` scope (a classic PAT or a GitHub App token with `Projects: read`).

The Markdown report can also be posted to a pull request with
[gh-comment-kit](https://github.com/srz-zumix/gh-comment-kit), which replaces the previous comment
of the same group instead of piling up new ones.

```sh
gh pm-kit projects lint 1 --owner my-org --exit-zero --summary-markdown lint.md
gh comment-kit review comment "$PR_NUMBER" --group project-lint --update --body-file lint.md
```

### projects migrate

Migrate a GitHub Project v2 (New Projects) from one owner to another.
Copies project metadata, the open/closed state, custom fields (TEXT, NUMBER, DATE, SINGLE_SELECT, MULTI_SELECT, ITERATION), views, items, item order, item archive state, and status updates.
Options of an existing destination SINGLE_SELECT or MULTI_SELECT field are aligned with the source: missing options are added, the color/description of matching options are refreshed, and source options are reordered to match the source so board layout columns keep the same order. Destination-only options are appended at the end.
MULTI_SELECT fields are skipped with a warning if their creation fails (for example, when the destination GitHub version does not support them), so the rest of the migration still completes.
ITERATION fields are recreated with both their past and current iterations, so sprints already completed in the source project are reproduced.
Views are recreated with their layout, filter, visible fields, sorting, and grouping; destination views with the same name are left untouched because the API has no view update endpoint, and destination views that do not exist in the source (such as the default view of a newly created project) are deleted.
Built-in workflows (automations) are not migrated: the API exposes only their name and enabled flag and offers no way to create or enable one. Enabled source workflows that are disabled in the destination are reported as a warning so they can be enabled manually.

The destination content of each item is resolved in the following order:

1. A destination item that already carries the migration marker.
2. An issue carrying the migration marker in the `--repo` repository.
3. The issue or pull request with the same repository name and number under the destination owner. Only the owner may differ between hosts, so the repository name and number must match, and the content type (issue or pull request) is verified before linking.
4. A new issue created in the `--repo` repository when `--create-issue` is set.
5. A draft issue.

Items linked to existing issues or pull requests are never deleted, even with `--overwrite`; only their field values are re-applied.
Archived items are migrated and archived again in the destination. Older GitHub Enterprise Server versions do not expose archived items through the API, so archived items of such a source project cannot be migrated.

If a destination project number or URL is given as the second argument, that project is used as the migration target.
Without a destination project, a new destination project is created when needed.

Items and status updates already migrated are identified by a hidden marker and skipped by default.

```sh
gh pm-kit projects migrate <number|URL> [dst-number|dst-URL] --dst OWNER [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-s, --src string` | current owner | Source owner in the format `[HOST/]OWNER` |
| `-d, --dst string` | **(required)** | Destination owner in the format `[HOST/]OWNER` (required unless a destination URL is given as the second argument) |
| `-r, --repo string` | | Repository in `[HOST/]OWNER/REPO` format; items are linked to matching issues (by migration marker) in this repository |
| `--create-issue` | `false` | When `--repo` is set and no existing issue or pull request matches, create a new issue instead of a draft issue |
| `--overwrite` | `false` | Overwrite previously migrated content identified by the migration marker: when no destination project is given, overwrite the existing migrated project instead of skipping it; migrated items are deleted and re-created, and migrated status updates are refreshed in place, instead of being skipped. Items linked to existing issues or pull requests are kept and only their field values are re-applied |

### projects stats

Show aggregated statistics for a GitHub Project v2.
The report covers item totals by type and state, custom field completeness, select/iteration value distribution (including options no item uses), repository, assignee and label distribution, view layouts, and an approximated lead time based on the issue `createdAt`/`closedAt` timestamps.
Archived items are excluded unless `--include-archived` is given; the archived count itself is always reported.
The project can be specified by its number or by its URL (e.g. `https://github.com/orgs/my-org/projects/1`).

```sh
gh pm-kit projects stats <number|URL> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Owner in the format `[HOST/]OWNER` |
| `--include-archived` | `false` | Include archived items in the statistics |
| `--group-by string` | | Custom field name to break items down by |
| `--format string` | | Output format: `json` |
| `-q, --jq expression` | | Filter JSON output using a jq expression |
| `-t, --template string` | | Format JSON output using a Go template |

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

A new Projects v2 project is created under the destination owner with the source name, body, and open/closed state.
Each column becomes an option in a `Column` single-select field, and a board view grouped by that field is created to mirror the classic layout.
Every card is migrated in its source order with the `Column` field set, and archived cards are migrated and archived again.
Note cards keep their note as the title and body; issue and pull-request cards are resolved on the source host so that their title and body are reproduced with a link back to the original.

Cards are migrated as draft issues by default.
If `--repo` is specified, the migration first searches for an existing issue carrying the migration marker in that repository and links it to the project.
If no matching issue is found and `--create-issue` is set, a new issue is created; otherwise a draft issue is used as a fallback.

Already-migrated items are identified by a hidden marker and skipped unless `--overwrite` is specified.

```sh
gh pm-kit projects v1 migrate <number|URL> --dst OWNER [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-o, --owner string` | current owner | Source owner in the format `[HOST/]OWNER`; inferred from URL if a project URL is given |
| `-R, --src-repo string` | | Source repository in the format `[HOST/]OWNER/REPO`; for repository-scoped classic projects; inferred from URL if a repo-scoped project URL is given |
| `-d, --dst string` | **(required)** | Destination owner in the format `[HOST/]OWNER` |
| `-r, --repo string` | | Repository in `[HOST/]OWNER/REPO` format; cards are linked to matching issues (by migration marker) in this repository |
| `--create-issue` | `false` | When `--repo` is set and no matching issue is found, create a new issue instead of a draft issue |
| `--overwrite` | `false` | Re-migrate already-migrated items instead of skipping them |
