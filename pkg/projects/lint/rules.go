package lint

import (
	"regexp"
	"strings"
	"time"

	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// rules holds every registered lint rule, ordered by rule ID.
var rules = []*ruleDef{
	{
		ID:              "PM001",
		Name:            "no-status",
		Description:     "Item has no value in the status field",
		DefaultSeverity: SeverityWarning,
		Check:           checkNoStatus,
	},
	{
		ID:              "PM002",
		Name:            "missing-required-field",
		Description:     "Item has no value in a field listed in --require",
		DefaultSeverity: SeverityWarning,
		Check:           checkMissingRequiredField,
	},
	{
		ID:              "PM003",
		Name:            "draft-issue-remaining",
		Description:     "Item is still a draft issue and is not tracked in a repository",
		DefaultSeverity: SeverityInfo,
		Check:           checkDraftIssueRemaining,
	},
	{
		ID:              "PM004",
		Name:            "stale-status-update",
		Description:     "No status update has been posted within --status-update-days",
		DefaultSeverity: SeverityWarning,
		Check:           checkStaleStatusUpdate,
	},
	{
		ID:              "PM005",
		Name:            "unused-select-option",
		Description:     "A select option is not used by any item",
		DefaultSeverity: SeverityInfo,
		Check:           checkUnusedSelectOption,
	},
	{
		ID:              "PM006",
		Name:            "broken-view-filter",
		Description:     "A view filter references an unknown qualifier or field",
		DefaultSeverity: SeverityInfo,
		Check:           checkBrokenViewFilter,
	},
	{
		ID:              "PM007",
		Name:            "duplicate-draft",
		Description:     "Multiple draft issues share the same title",
		DefaultSeverity: SeverityWarning,
		Check:           checkDuplicateDraft,
	},
	{
		ID:              "PM010",
		Name:            "closed-issue-not-done",
		Description:     "Linked issue or pull request is closed but the status is not a done status",
		DefaultSeverity: SeverityError,
		Check:           checkClosedIssueNotDone,
	},
	{
		ID:              "PM011",
		Name:            "done-but-open",
		Description:     "Status is a done status but the linked issue or pull request is still open",
		DefaultSeverity: SeverityError,
		Check:           checkDoneButOpen,
	},
	{
		ID:              "PM012",
		Name:            "stale-item",
		Description:     "Unfinished item has not been updated within --stale-days",
		DefaultSeverity: SeverityWarning,
		Check:           checkStaleItem,
	},
	{
		ID:              "PM013",
		Name:            "unassigned-in-progress",
		Description:     "Item is in an in-progress status but has no assignee",
		DefaultSeverity: SeverityWarning,
		Check:           checkUnassignedInProgress,
	},
	{
		ID:              "PM014",
		Name:            "archived-but-open",
		Description:     "Item is archived but the linked issue or pull request is still open",
		DefaultSeverity: SeverityWarning,
		Check:           checkArchivedButOpen,
	},
	{
		ID:              "PM015",
		Name:            "past-iteration-incomplete",
		Description:     "Item is still assigned to a completed iteration but is not finished",
		DefaultSeverity: SeverityWarning,
		Check:           checkPastIterationIncomplete,
	},
	{
		ID:              "PM016",
		Name:            "orphaned-item",
		Description:     "Linked issue or pull request is no longer accessible",
		DefaultSeverity: SeverityError,
		Check:           checkOrphanedItem,
	},
}

func checkNoStatus(c *Context, r *reporter) {
	if c.statusField == nil {
		return
	}
	for _, item := range c.Items {
		if c.StatusOf(item) == "" {
			r.item(item, "field %q is not set", c.statusField.Name)
		}
	}
}

func checkMissingRequiredField(c *Context, r *reporter) {
	for _, name := range c.Options.RequiredFields {
		field := c.Project.FindField(name)
		if field == nil {
			r.project("required field %q does not exist in the project", name)
			continue
		}
		for _, item := range c.Items {
			if len(projects.ItemFieldValues(item, field.Name)) == 0 {
				r.item(item, "field %q is not set", field.Name)
			}
		}
	}
}

func checkDraftIssueRemaining(c *Context, r *reporter) {
	for _, item := range c.Items {
		if item.Content.Type == gh.ProjectV2ItemTypeDraftIssue {
			r.item(item, "item is still a draft issue")
		}
	}
}

func checkStaleStatusUpdate(c *Context, r *reporter) {
	latest := projects.LatestStatusUpdate(c.Project.StatusUpdates)
	if latest == nil {
		r.project("no status update has been posted")
		return
	}
	postedAt, err := time.Parse(time.RFC3339, latest.CreatedAt)
	if err != nil {
		return
	}
	days := int(c.now.Sub(postedAt).Hours() / 24)
	if days > c.Options.StatusUpdateDays {
		r.project("the latest status update is %d days old (allowed: %d)", days, c.Options.StatusUpdateDays)
	}
}

func checkUnusedSelectOption(c *Context, r *reporter) {
	for i := range c.Project.Fields {
		field := &c.Project.Fields[i]
		if field.DataType != "SINGLE_SELECT" && field.DataType != "MULTI_SELECT" {
			continue
		}
		used := map[string]bool{}
		for _, item := range c.Items {
			for _, v := range projects.ItemFieldValues(item, field.Name) {
				used[v] = true
			}
		}
		for _, option := range field.Options {
			if !used[option.Name] {
				r.project("option %q of field %q is not used by any item", option.Name, field.Name)
			}
		}
	}
}

// viewFilterQualifier matches the "qualifier:" prefixes of a project view filter.
var viewFilterQualifier = regexp.MustCompile(`(?:^|\s)-?([A-Za-z][A-Za-z0-9_-]*):`)

// viewFilterBuiltins are the qualifiers GitHub understands in addition to the
// project's own field names.
var viewFilterBuiltins = []string{
	"is", "no", "has", "in", "sort", "label", "assignee", "author", "reviewer",
	"milestone", "repo", "org", "project", "type", "state", "status", "linked",
	"parent-issue", "sub-issue", "updated", "created", "closed", "merged",
	"last-updated", "reason", "comments", "interactions", "draft", "review",
	"team", "mentions", "involves", "commenter",
}

func checkBrokenViewFilter(c *Context, r *reporter) {
	known := map[string]bool{}
	for _, name := range viewFilterBuiltins {
		known[normalizeQualifier(name)] = true
	}
	for i := range c.Project.Fields {
		known[normalizeQualifier(c.Project.Fields[i].Name)] = true
	}

	for _, view := range c.Project.Views {
		if view.Filter == "" {
			continue
		}
		reported := map[string]bool{}
		for _, match := range viewFilterQualifier.FindAllStringSubmatch(view.Filter, -1) {
			qualifier := match[1]
			normalized := normalizeQualifier(qualifier)
			if known[normalized] || reported[normalized] {
				continue
			}
			reported[normalized] = true
			r.project("view %q filters on unknown qualifier %q", view.Name, qualifier)
		}
	}
}

// normalizeQualifier makes field names and filter qualifiers comparable by
// dropping the separators GitHub allows between words.
func normalizeQualifier(s string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return strings.ToLower(replacer.Replace(s))
}

func checkDuplicateDraft(c *Context, r *reporter) {
	seen := map[string]bool{}
	for _, item := range c.Items {
		if item.Content.Type != gh.ProjectV2ItemTypeDraftIssue {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Content.Title))
		if key == "" {
			continue
		}
		if seen[key] {
			r.item(item, "another draft issue already uses the title %q", item.Content.Title)
			continue
		}
		seen[key] = true
	}
}

func checkClosedIssueNotDone(c *Context, r *reporter) {
	if c.statusField == nil {
		return
	}
	for _, item := range c.Items {
		if !isClosedState(item.Content.State) {
			continue
		}
		status := c.StatusOf(item)
		if status == "" || c.IsDoneStatus(status) {
			continue
		}
		r.item(item, "%s is %s but the status is %q", contentNoun(item), strings.ToLower(item.Content.State), status)
	}
}

func checkDoneButOpen(c *Context, r *reporter) {
	if c.statusField == nil {
		return
	}
	for _, item := range c.Items {
		if !item.Content.IsOpen() {
			continue
		}
		status := c.StatusOf(item)
		if status == "" || !c.IsDoneStatus(status) {
			continue
		}
		r.item(item, "the status is %q but the %s is still open", status, contentNoun(item))
	}
}

func checkStaleItem(c *Context, r *reporter) {
	for _, item := range c.Items {
		if c.IsDone(item) {
			continue
		}
		updatedAt, err := time.Parse(time.RFC3339, item.Content.UpdatedAt)
		if err != nil {
			continue
		}
		days := int(c.now.Sub(updatedAt).Hours() / 24)
		if days > c.Options.StaleDays {
			r.item(item, "not updated for %d days (allowed: %d)", days, c.Options.StaleDays)
		}
	}
}

func checkUnassignedInProgress(c *Context, r *reporter) {
	if c.statusField == nil {
		return
	}
	for _, item := range c.Items {
		if item.Content.Type == gh.ProjectV2ItemTypeDraftIssue {
			continue
		}
		status := c.StatusOf(item)
		if !c.IsInProgressStatus(status) {
			continue
		}
		if len(item.Content.Assignees) == 0 {
			r.item(item, "the status is %q but nobody is assigned", status)
		}
	}
}

func checkArchivedButOpen(c *Context, r *reporter) {
	for _, item := range c.Project.Items {
		if item.IsArchived && item.Content.IsOpen() {
			r.item(item, "item is archived but the %s is still open", contentNoun(item))
		}
	}
}

func checkPastIterationIncomplete(c *Context, r *reporter) {
	for i := range c.Project.Fields {
		field := &c.Project.Fields[i]
		if field.DataType != "ITERATION" || len(field.CompletedIterations) == 0 {
			continue
		}
		completed := map[string]bool{}
		for _, iteration := range field.CompletedIterations {
			completed[iteration.Title] = true
		}
		for _, item := range c.Items {
			if c.IsDone(item) {
				continue
			}
			for _, value := range projects.ItemFieldValues(item, field.Name) {
				if completed[value] {
					r.item(item, "iteration %q of field %q has ended but the item is not done", value, field.Name)
				}
			}
		}
	}
}

func checkOrphanedItem(c *Context, r *reporter) {
	for _, item := range c.Items {
		if item.Content.Type == gh.ProjectV2ItemTypeRedacted {
			r.item(item, "the linked issue or pull request is no longer accessible")
		}
	}
}

// contentNoun names the linked content of an item for use in messages.
func contentNoun(item gh.ProjectV2Item) string {
	if item.Content.Type == gh.ProjectV2ItemTypePullRequest {
		return "pull request"
	}
	return "issue"
}
