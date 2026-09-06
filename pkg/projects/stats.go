package projects

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// StatsOptions controls how project statistics are calculated.
type StatsOptions struct {
	// IncludeArchived counts archived items in every section except the archived total.
	IncludeArchived bool
	// GroupBy is the name of a custom field to break items down by. Empty disables the breakdown.
	GroupBy string
}

// CalcProjectStats aggregates statistics for a collected project.
func CalcProjectStats(collected *CollectedProject, opts StatsOptions) (*render.ProjectStatsReport, error) {
	groupField := ""
	if opts.GroupBy != "" {
		field := collected.FindField(opts.GroupBy)
		if field == nil {
			return nil, fmt.Errorf("field %q does not exist in project #%d of '%s'", opts.GroupBy, collected.Number, collected.Owner)
		}
		groupField = field.Name
	}

	items := make([]gh.ProjectV2Item, 0, len(collected.Items))
	archived := 0
	for _, item := range collected.Items {
		if item.IsArchived {
			archived++
			if !opts.IncludeArchived {
				continue
			}
		}
		items = append(items, item)
	}

	report := &render.ProjectStatsReport{
		Label:        collected.Label(),
		Title:        collected.Project.Title,
		Summary:      calcProjectStatsSummary(collected, items, archived),
		Fields:       calcProjectFieldStats(collected.Fields, items),
		GroupBy:      groupField,
		Repositories: calcProjectStatsCounts(items, projectItemRepositories),
		Assignees:    calcProjectStatsCounts(items, projectItemAssignees),
		Labels:       calcProjectStatsCounts(items, projectItemLabels),
		ViewLayouts:  calcProjectViewLayoutStats(collected.Views),
		LeadTime:     calcProjectLeadTimeStats(items),
	}
	if groupField != "" {
		report.Groups = calcProjectGroupStats(items, groupField)
	}
	return report, nil
}

func calcProjectStatsSummary(collected *CollectedProject, items []gh.ProjectV2Item, archived int) render.ProjectStatsSummary {
	summary := render.ProjectStatsSummary{
		TotalItems:    len(collected.Items),
		CountedItems:  len(items),
		ArchivedItems: archived,
		Views:         len(collected.Views),
		StatusUpdates: len(collected.StatusUpdates),
	}
	for _, item := range items {
		switch item.Content.Type {
		case gh.ProjectV2ItemTypeIssue:
			summary.Issues++
		case gh.ProjectV2ItemTypePullRequest:
			summary.PullRequests++
		case gh.ProjectV2ItemTypeDraftIssue:
			summary.DraftIssues++
		}
		switch {
		case item.Content.IsOpen():
			summary.OpenItems++
		case item.Content.State != "":
			summary.ClosedItems++
		}
	}
	if latest := LatestStatusUpdate(collected.StatusUpdates); latest != nil {
		summary.LatestStatus = string(latest.Status)
		summary.LatestStatusAt = latest.CreatedAt
	}
	return summary
}

// LatestStatusUpdate returns the most recently created status update.
func LatestStatusUpdate(updates []gh.ProjectV2StatusUpdate) *gh.ProjectV2StatusUpdate {
	var latest *gh.ProjectV2StatusUpdate
	var latestAt time.Time
	for i := range updates {
		at, err := time.Parse(time.RFC3339, updates[i].CreatedAt)
		if err != nil {
			continue
		}
		if latest == nil || at.After(latestAt) {
			latest = &updates[i]
			latestAt = at
		}
	}
	return latest
}

func calcProjectFieldStats(fields []gh.ProjectV2Field, items []gh.ProjectV2Item) []render.ProjectFieldStats {
	var stats []render.ProjectFieldStats
	for i := range fields {
		field := &fields[i]
		if !migratableDataTypes[field.DataType] {
			continue
		}

		counts := map[string]int{}
		set := 0
		for _, item := range items {
			values := ItemFieldValues(item, field.Name)
			if len(values) == 0 {
				continue
			}
			set++
			for _, v := range values {
				counts[v]++
			}
		}

		stat := render.ProjectFieldStats{
			Name:         field.Name,
			DataType:     field.DataType,
			SetItems:     set,
			UnsetItems:   len(items) - set,
			Completeness: percent(set, len(items)),
			Values:       fieldValueDistribution(field, counts, len(items)),
		}
		stats = append(stats, stat)
	}
	return stats
}

// fieldValueDistribution builds the value distribution of a select or iteration
// field. Defined options that no item uses are kept with a zero count so that
// unused options stay visible.
func fieldValueDistribution(field *gh.ProjectV2Field, counts map[string]int, total int) []render.ProjectStatsCount {
	var names []string
	switch field.DataType {
	case "SINGLE_SELECT", "MULTI_SELECT":
		for _, o := range field.Options {
			names = append(names, o.Name)
		}
	case "ITERATION":
		for _, it := range field.AllIterations() {
			names = append(names, it.Title)
		}
	default:
		return nil
	}

	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	var extra []string
	for name := range counts {
		if !known[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	names = append(names, extra...)

	values := make([]render.ProjectStatsCount, 0, len(names))
	for _, name := range names {
		values = append(values, render.ProjectStatsCount{
			Name:    name,
			Count:   counts[name],
			Percent: percent(counts[name], total),
		})
	}
	return values
}

func calcProjectGroupStats(items []gh.ProjectV2Item, fieldName string) []render.ProjectGroupStats {
	const noValue = "(none)"
	order := []string{}
	byName := map[string]*render.ProjectGroupStats{}

	add := func(name string, item gh.ProjectV2Item) {
		group, ok := byName[name]
		if !ok {
			group = &render.ProjectGroupStats{Name: name}
			byName[name] = group
			order = append(order, name)
		}
		group.Count++
		if item.IsArchived {
			group.Archived++
		}
		switch {
		case item.Content.IsOpen():
			group.Open++
		case item.Content.State != "":
			group.Closed++
		}
	}

	for _, item := range items {
		values := ItemFieldValues(item, fieldName)
		if len(values) == 0 {
			add(noValue, item)
			continue
		}
		for _, v := range values {
			add(v, item)
		}
	}

	groups := make([]render.ProjectGroupStats, 0, len(order))
	for _, name := range order {
		groups = append(groups, *byName[name])
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	return groups
}

func calcProjectStatsCounts(items []gh.ProjectV2Item, values func(gh.ProjectV2Item) []string) []render.ProjectStatsCount {
	counts := map[string]int{}
	for _, item := range items {
		for _, v := range values(item) {
			counts[v]++
		}
	}
	return sortedStatsCounts(counts, len(items))
}

func calcProjectViewLayoutStats(views []gh.ProjectV2View) []render.ProjectStatsCount {
	counts := map[string]int{}
	for _, v := range views {
		counts[strings.TrimSuffix(v.Layout, "_LAYOUT")]++
	}
	return sortedStatsCounts(counts, len(views))
}

// calcProjectLeadTimeStats approximates lead time from the createdAt/closedAt
// timestamps of closed issues and pull requests. Projects v2 does not expose
// status transition history, so this is the closest available measure.
func calcProjectLeadTimeStats(items []gh.ProjectV2Item) *render.ProjectLeadTimeStats {
	var days []float64
	for _, item := range items {
		created, err := time.Parse(time.RFC3339, item.Content.CreatedAt)
		if err != nil {
			continue
		}
		closed, err := time.Parse(time.RFC3339, item.Content.ClosedAt)
		if err != nil {
			continue
		}
		if closed.Before(created) {
			continue
		}
		days = append(days, closed.Sub(created).Hours()/24)
	}
	if len(days) == 0 {
		return nil
	}
	sort.Float64s(days)

	sum := 0.0
	for _, d := range days {
		sum += d
	}
	median := days[len(days)/2]
	if len(days)%2 == 0 {
		median = (days[len(days)/2-1] + days[len(days)/2]) / 2
	}
	return &render.ProjectLeadTimeStats{
		Items:       len(days),
		MedianDays:  median,
		AverageDays: sum / float64(len(days)),
		MinDays:     days[0],
		MaxDays:     days[len(days)-1],
	}
}

func projectItemRepositories(item gh.ProjectV2Item) []string {
	if item.Content.RepoOwner == "" {
		return nil
	}
	return []string{item.Content.RepoOwner + "/" + item.Content.RepoName}
}

func projectItemAssignees(item gh.ProjectV2Item) []string {
	return item.Content.Assignees
}

func projectItemLabels(item gh.ProjectV2Item) []string {
	return item.Content.Labels
}

// ItemFieldValues returns the display values an item holds for the named
// field. Multi-select fields yield one entry per selected option.
func ItemFieldValues(item gh.ProjectV2Item, fieldName string) []string {
	for _, fv := range item.FieldValues {
		if !strings.EqualFold(fv.FieldName, fieldName) {
			continue
		}
		switch fv.ValueType {
		case "TEXT":
			return nonEmpty(fv.Text)
		case "NUMBER":
			if fv.Number == nil {
				return nil
			}
			return []string{strconv.FormatFloat(*fv.Number, 'g', -1, 64)}
		case "DATE":
			return nonEmpty(fv.Date)
		case "SINGLE_SELECT":
			return nonEmpty(fv.SelectName)
		case "MULTI_SELECT":
			return fv.SelectNames
		case "ITERATION":
			return nonEmpty(fv.IterationTitle)
		}
	}
	return nil
}

func nonEmpty(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}

func sortedStatsCounts(counts map[string]int, total int) []render.ProjectStatsCount {
	result := make([]render.ProjectStatsCount, 0, len(counts))
	for name, count := range counts {
		result = append(result, render.ProjectStatsCount{
			Name:    name,
			Count:   count,
			Percent: percent(count, total),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func percent(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) * 100 / float64(total)
}
