package projects

import (
	"testing"

	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

func statsTestProject() *CollectedProject {
	return &CollectedProject{
		Host:    "github.com",
		Owner:   "octo",
		Number:  1,
		Project: &gh.ProjectV2{Number: 1, Title: "Roadmap"},
		Fields: []gh.ProjectV2Field{
			{
				Name:     "Status",
				DataType: "SINGLE_SELECT",
				Options: []gh.ProjectV2SelectOption{
					{Name: "Todo"},
					{Name: "In Progress"},
					{Name: "Done"},
				},
			},
			{Name: "Title", DataType: "TITLE"},
		},
		Items: []gh.ProjectV2Item{
			{
				ID:         "i1",
				IsArchived: false,
				Content: gh.ProjectV2ItemContent{
					Type:      gh.ProjectV2ItemTypeIssue,
					State:     "OPEN",
					RepoOwner: "octo",
					RepoName:  "api",
					Assignees: []string{"alice"},
					Labels:    []string{"bug"},
					CreatedAt: "2026-01-01T00:00:00Z",
				},
				FieldValues: []gh.ProjectV2FieldValue{
					{FieldName: "Status", ValueType: "SINGLE_SELECT", SelectName: "Todo"},
				},
			},
			{
				ID:         "i2",
				IsArchived: false,
				Content: gh.ProjectV2ItemContent{
					Type:      gh.ProjectV2ItemTypeIssue,
					State:     "CLOSED",
					RepoOwner: "octo",
					RepoName:  "api",
					Assignees: []string{"alice", "bob"},
					Labels:    []string{"bug"},
					CreatedAt: "2026-01-01T00:00:00Z",
					ClosedAt:  "2026-01-05T00:00:00Z",
				},
				FieldValues: []gh.ProjectV2FieldValue{
					{FieldName: "Status", ValueType: "SINGLE_SELECT", SelectName: "Todo"},
				},
			},
			{
				ID:         "i3",
				IsArchived: true,
				Content: gh.ProjectV2ItemContent{
					Type:      gh.ProjectV2ItemTypePullRequest,
					State:     "MERGED",
					RepoOwner: "octo",
					RepoName:  "web",
					CreatedAt: "2026-01-01T00:00:00Z",
					ClosedAt:  "2026-01-03T00:00:00Z",
				},
			},
			{
				ID:      "i4",
				Content: gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeDraftIssue, Title: "Spike"},
			},
		},
		Views: []gh.ProjectV2View{
			{Name: "Board", Layout: "BOARD_LAYOUT"},
			{Name: "Table", Layout: "TABLE_LAYOUT"},
		},
		StatusUpdates: []gh.ProjectV2StatusUpdate{
			{Status: "ON_TRACK", CreatedAt: "2026-01-02T00:00:00Z"},
			{Status: "AT_RISK", CreatedAt: "2026-02-02T00:00:00Z"},
			{Status: "COMPLETE", CreatedAt: "2026-01-20T00:00:00Z"},
		},
	}
}

func TestCalcProjectStatsSummaryExcludesArchived(t *testing.T) {
	report, err := CalcProjectStats(statsTestProject(), StatsOptions{})
	if err != nil {
		t.Fatalf("CalcProjectStats returned error: %v", err)
	}

	s := report.Summary
	if s.TotalItems != 4 {
		t.Errorf("TotalItems = %d, want 4", s.TotalItems)
	}
	if s.CountedItems != 3 {
		t.Errorf("CountedItems = %d, want 3", s.CountedItems)
	}
	if s.ArchivedItems != 1 {
		t.Errorf("ArchivedItems = %d, want 1", s.ArchivedItems)
	}
	if s.Issues != 2 || s.PullRequests != 0 || s.DraftIssues != 1 {
		t.Errorf("type counts = (%d, %d, %d), want (2, 0, 1)", s.Issues, s.PullRequests, s.DraftIssues)
	}
	if s.OpenItems != 1 || s.ClosedItems != 1 {
		t.Errorf("open/closed = (%d, %d), want (1, 1)", s.OpenItems, s.ClosedItems)
	}
	if s.Views != 2 || s.StatusUpdates != 3 {
		t.Errorf("views/statusUpdates = (%d, %d), want (2, 3)", s.Views, s.StatusUpdates)
	}
	if s.LatestStatus != "AT_RISK" {
		t.Errorf("LatestStatus = %q, want AT_RISK", s.LatestStatus)
	}
}

func TestCalcProjectStatsIncludeArchived(t *testing.T) {
	report, err := CalcProjectStats(statsTestProject(), StatsOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("CalcProjectStats returned error: %v", err)
	}
	if report.Summary.CountedItems != 4 {
		t.Errorf("CountedItems = %d, want 4", report.Summary.CountedItems)
	}
	if report.Summary.PullRequests != 1 {
		t.Errorf("PullRequests = %d, want 1", report.Summary.PullRequests)
	}
	if report.Summary.ClosedItems != 2 {
		t.Errorf("ClosedItems = %d, want 2", report.Summary.ClosedItems)
	}
}

func TestCalcProjectStatsFieldsKeepUnusedOptions(t *testing.T) {
	report, err := CalcProjectStats(statsTestProject(), StatsOptions{})
	if err != nil {
		t.Fatalf("CalcProjectStats returned error: %v", err)
	}

	if len(report.Fields) != 1 {
		t.Fatalf("Fields length = %d, want 1 (only custom data types)", len(report.Fields))
	}
	status := report.Fields[0]
	if status.Name != "Status" {
		t.Fatalf("Fields[0].Name = %q, want Status", status.Name)
	}
	if status.SetItems != 2 || status.UnsetItems != 1 {
		t.Errorf("set/unset = (%d, %d), want (2, 1)", status.SetItems, status.UnsetItems)
	}

	want := map[string]int{"Todo": 2, "In Progress": 0, "Done": 0}
	if len(status.Values) != len(want) {
		t.Fatalf("Values length = %d, want %d", len(status.Values), len(want))
	}
	for _, v := range status.Values {
		if got, ok := want[v.Name]; !ok || got != v.Count {
			t.Errorf("Values[%q] = %d, want %d", v.Name, v.Count, want[v.Name])
		}
	}
}

func TestCalcProjectStatsGroupBy(t *testing.T) {
	report, err := CalcProjectStats(statsTestProject(), StatsOptions{GroupBy: "status"})
	if err != nil {
		t.Fatalf("CalcProjectStats returned error: %v", err)
	}
	if report.GroupBy != "Status" {
		t.Errorf("GroupBy = %q, want Status (resolved to the defined field name)", report.GroupBy)
	}
	if len(report.Groups) != 2 {
		t.Fatalf("Groups length = %d, want 2", len(report.Groups))
	}
	if report.Groups[0].Name != "Todo" || report.Groups[0].Count != 2 {
		t.Errorf("Groups[0] = %+v, want Todo with 2 items", report.Groups[0])
	}
	if report.Groups[0].Open != 1 || report.Groups[0].Closed != 1 {
		t.Errorf("Groups[0] open/closed = (%d, %d), want (1, 1)", report.Groups[0].Open, report.Groups[0].Closed)
	}
	if report.Groups[1].Name != "(none)" || report.Groups[1].Count != 1 {
		t.Errorf("Groups[1] = %+v, want (none) with 1 item", report.Groups[1])
	}
}

func TestCalcProjectStatsGroupByUnknownField(t *testing.T) {
	if _, err := CalcProjectStats(statsTestProject(), StatsOptions{GroupBy: "Priority"}); err == nil {
		t.Fatal("CalcProjectStats should fail for an unknown group-by field")
	}
}

func TestCalcProjectStatsLeadTime(t *testing.T) {
	report, err := CalcProjectStats(statsTestProject(), StatsOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("CalcProjectStats returned error: %v", err)
	}
	if report.LeadTime == nil {
		t.Fatal("LeadTime is nil, want statistics for the two closed items")
	}
	if report.LeadTime.Items != 2 {
		t.Errorf("LeadTime.Items = %d, want 2", report.LeadTime.Items)
	}
	if report.LeadTime.MinDays != 2 || report.LeadTime.MaxDays != 4 {
		t.Errorf("LeadTime min/max = (%v, %v), want (2, 4)", report.LeadTime.MinDays, report.LeadTime.MaxDays)
	}
	if report.LeadTime.AverageDays != 3 {
		t.Errorf("LeadTime.AverageDays = %v, want 3", report.LeadTime.AverageDays)
	}
}

func TestCalcProjectStatsDistributions(t *testing.T) {
	report, err := CalcProjectStats(statsTestProject(), StatsOptions{})
	if err != nil {
		t.Fatalf("CalcProjectStats returned error: %v", err)
	}
	if len(report.Repositories) != 1 || report.Repositories[0].Name != "octo/api" || report.Repositories[0].Count != 2 {
		t.Errorf("Repositories = %+v, want octo/api with 2 items", report.Repositories)
	}
	if len(report.Assignees) != 2 || report.Assignees[0].Name != "alice" || report.Assignees[0].Count != 2 {
		t.Errorf("Assignees = %+v, want alice first with 2 items", report.Assignees)
	}
	if len(report.Labels) != 1 || report.Labels[0].Count != 2 {
		t.Errorf("Labels = %+v, want bug with 2 items", report.Labels)
	}
	if len(report.ViewLayouts) != 2 {
		t.Errorf("ViewLayouts = %+v, want 2 layouts", report.ViewLayouts)
	}
}
