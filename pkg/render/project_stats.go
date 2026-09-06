package render

import (
	"fmt"
	"strconv"

	ghrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

// ProjectStatsCount is a labelled count together with its share of the counted items.
type ProjectStatsCount struct {
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// ProjectStatsSummary holds the project-wide totals.
type ProjectStatsSummary struct {
	TotalItems     int    `json:"totalItems"`
	CountedItems   int    `json:"countedItems"`
	ArchivedItems  int    `json:"archivedItems"`
	Issues         int    `json:"issues"`
	PullRequests   int    `json:"pullRequests"`
	DraftIssues    int    `json:"draftIssues"`
	OpenItems      int    `json:"openItems"`
	ClosedItems    int    `json:"closedItems"`
	Views          int    `json:"views"`
	StatusUpdates  int    `json:"statusUpdates"`
	LatestStatus   string `json:"latestStatus,omitempty"`
	LatestStatusAt string `json:"latestStatusAt,omitempty"`
}

// ProjectFieldStats holds the completeness and value distribution of one custom field.
type ProjectFieldStats struct {
	Name         string  `json:"name"`
	DataType     string  `json:"dataType"`
	SetItems     int     `json:"setItems"`
	UnsetItems   int     `json:"unsetItems"`
	Completeness float64 `json:"completeness"`
	// Values is populated for SINGLE_SELECT, MULTI_SELECT and ITERATION fields and
	// includes defined options that no item uses.
	Values []ProjectStatsCount `json:"values,omitempty"`
}

// ProjectGroupStats is one row of the --group-by breakdown.
type ProjectGroupStats struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Open     int    `json:"open"`
	Closed   int    `json:"closed"`
	Archived int    `json:"archived"`
}

// ProjectLeadTimeStats approximates lead time from the issue createdAt/closedAt timestamps.
type ProjectLeadTimeStats struct {
	Items       int     `json:"items"`
	MedianDays  float64 `json:"medianDays"`
	AverageDays float64 `json:"averageDays"`
	MinDays     float64 `json:"minDays"`
	MaxDays     float64 `json:"maxDays"`
}

// ProjectStatsReport is the top-level report for rendering project statistics.
type ProjectStatsReport struct {
	Label        string                `json:"label"`
	Title        string                `json:"title"`
	Summary      ProjectStatsSummary   `json:"summary"`
	Fields       []ProjectFieldStats   `json:"fields,omitempty"`
	GroupBy      string                `json:"groupBy,omitempty"`
	Groups       []ProjectGroupStats   `json:"groups,omitempty"`
	Repositories []ProjectStatsCount   `json:"repositories,omitempty"`
	Assignees    []ProjectStatsCount   `json:"assignees,omitempty"`
	Labels       []ProjectStatsCount   `json:"labels,omitempty"`
	ViewLayouts  []ProjectStatsCount   `json:"viewLayouts,omitempty"`
	LeadTime     *ProjectLeadTimeStats `json:"leadTime,omitempty"`
}

// RenderProjectStats renders a ProjectStatsReport using the provided Renderer.
func RenderProjectStats(r *ghrender.Renderer, report *ProjectStatsReport) error {
	if r.HasExporter() {
		return r.RenderExportedData(report)
	}

	r.WriteLine(fmt.Sprintf("%s %s", report.Label, report.Title))

	if err := renderProjectStatsSummary(r, &report.Summary); err != nil {
		return err
	}
	if err := renderProjectFieldStats(r, report.Fields); err != nil {
		return err
	}
	if err := renderProjectFieldValueStats(r, report.Fields); err != nil {
		return err
	}
	if err := renderProjectGroupStats(r, report.GroupBy, report.Groups); err != nil {
		return err
	}
	if err := renderProjectStatsCounts(r, "Repositories", "REPOSITORY", report.Repositories); err != nil {
		return err
	}
	if err := renderProjectStatsCounts(r, "Assignees", "ASSIGNEE", report.Assignees); err != nil {
		return err
	}
	if err := renderProjectStatsCounts(r, "Labels", "LABEL", report.Labels); err != nil {
		return err
	}
	if err := renderProjectStatsCounts(r, "Views", "LAYOUT", report.ViewLayouts); err != nil {
		return err
	}
	return renderProjectLeadTimeStats(r, report.LeadTime)
}

func renderProjectStatsSummary(r *ghrender.Renderer, s *ProjectStatsSummary) error {
	rows := [][]string{
		{"Total items", strconv.Itoa(s.TotalItems)},
		{"Counted items", strconv.Itoa(s.CountedItems)},
		{"Archived items", strconv.Itoa(s.ArchivedItems)},
		{"Issues", strconv.Itoa(s.Issues)},
		{"Pull requests", strconv.Itoa(s.PullRequests)},
		{"Draft issues", strconv.Itoa(s.DraftIssues)},
		{"Open", strconv.Itoa(s.OpenItems)},
		{"Closed", strconv.Itoa(s.ClosedItems)},
		{"Views", strconv.Itoa(s.Views)},
		{"Status updates", strconv.Itoa(s.StatusUpdates)},
	}
	if s.LatestStatus != "" || s.LatestStatusAt != "" {
		rows = append(rows, []string{"Latest status", s.LatestStatus}, []string{"Latest status at", s.LatestStatusAt})
	}

	r.WriteLine("")
	r.WriteLine("Summary")
	table := r.NewTableWriter([]string{"METRIC", "VALUE"})
	for _, row := range rows {
		table.Append(row)
	}
	return table.Render()
}

func renderProjectFieldStats(r *ghrender.Renderer, fields []ProjectFieldStats) error {
	if len(fields) == 0 {
		return nil
	}
	r.WriteLine("")
	r.WriteLine("Field completeness")
	table := r.NewTableWriter([]string{"FIELD", "DATA TYPE", "SET", "UNSET", "COMPLETENESS"})
	for _, f := range fields {
		table.Append([]string{
			f.Name,
			f.DataType,
			strconv.Itoa(f.SetItems),
			strconv.Itoa(f.UnsetItems),
			formatPercent(f.Completeness),
		})
	}
	return table.Render()
}

func renderProjectFieldValueStats(r *ghrender.Renderer, fields []ProjectFieldStats) error {
	var withValues []ProjectFieldStats
	for _, f := range fields {
		if len(f.Values) > 0 {
			withValues = append(withValues, f)
		}
	}
	if len(withValues) == 0 {
		return nil
	}

	r.WriteLine("")
	r.WriteLine("Field values")
	table := r.NewTableWriter([]string{"FIELD", "VALUE", "ITEMS", "SHARE"})
	for _, f := range withValues {
		for _, v := range f.Values {
			table.Append([]string{f.Name, v.Name, strconv.Itoa(v.Count), formatPercent(v.Percent)})
		}
	}
	return table.Render()
}

func renderProjectGroupStats(r *ghrender.Renderer, groupBy string, groups []ProjectGroupStats) error {
	if groupBy == "" {
		return nil
	}
	r.WriteLine("")
	r.WriteLine(fmt.Sprintf("Grouped by %q", groupBy))
	if len(groups) == 0 {
		r.WriteLine("No items.")
		return nil
	}
	table := r.NewTableWriter([]string{"VALUE", "ITEMS", "OPEN", "CLOSED", "ARCHIVED"})
	for _, g := range groups {
		table.Append([]string{
			g.Name,
			strconv.Itoa(g.Count),
			strconv.Itoa(g.Open),
			strconv.Itoa(g.Closed),
			strconv.Itoa(g.Archived),
		})
	}
	return table.Render()
}

func renderProjectStatsCounts(r *ghrender.Renderer, section, header string, counts []ProjectStatsCount) error {
	if len(counts) == 0 {
		return nil
	}
	r.WriteLine("")
	r.WriteLine(section)
	table := r.NewTableWriter([]string{header, "ITEMS", "SHARE"})
	for _, c := range counts {
		table.Append([]string{c.Name, strconv.Itoa(c.Count), formatPercent(c.Percent)})
	}
	return table.Render()
}

func renderProjectLeadTimeStats(r *ghrender.Renderer, lt *ProjectLeadTimeStats) error {
	if lt == nil {
		return nil
	}
	r.WriteLine("")
	r.WriteLine("Lead time (created to closed)")
	table := r.NewTableWriter([]string{"METRIC", "VALUE"})
	table.Append([]string{"Closed items measured", strconv.Itoa(lt.Items)})
	table.Append([]string{"Median", formatDays(lt.MedianDays)})
	table.Append([]string{"Average", formatDays(lt.AverageDays)})
	table.Append([]string{"Min", formatDays(lt.MinDays)})
	table.Append([]string{"Max", formatDays(lt.MaxDays)})
	return table.Render()
}

func formatPercent(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

func formatDays(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + "d"
}
