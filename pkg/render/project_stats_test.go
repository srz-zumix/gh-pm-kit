package render

import (
	"strings"
	"testing"

	ghrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

func TestRenderProjectStats(t *testing.T) {
	r := ghrender.NewStringRenderer(nil)
	report := &ProjectStatsReport{
		Label: "#1 octo",
		Title: "Roadmap",
		Summary: ProjectStatsSummary{
			TotalItems: 4, CountedItems: 3, ArchivedItems: 1,
			Issues: 2, DraftIssues: 1, OpenItems: 1, ClosedItems: 1,
			Views: 2, StatusUpdates: 3,
			LatestStatus: "AT_RISK", LatestStatusAt: "2026-02-02T00:00:00Z",
		},
		Fields: []ProjectFieldStats{{
			Name: "Status", DataType: "SINGLE_SELECT",
			SetItems: 2, UnsetItems: 1, Completeness: 66.7,
			Values: []ProjectStatsCount{
				{Name: "Todo", Count: 2, Percent: 66.7},
				{Name: "Done", Count: 0},
			},
		}},
		GroupBy:      "Status",
		Groups:       []ProjectGroupStats{{Name: "Todo", Count: 2, Open: 1, Closed: 1}},
		Repositories: []ProjectStatsCount{{Name: "octo/api", Count: 2, Percent: 66.7}},
		ViewLayouts:  []ProjectStatsCount{{Name: "BOARD", Count: 1, Percent: 50}},
		LeadTime:     &ProjectLeadTimeStats{Items: 2, MedianDays: 3, AverageDays: 3, MinDays: 2, MaxDays: 4},
	}

	if err := RenderProjectStats(&r.Renderer, report); err != nil {
		t.Fatalf("RenderProjectStats returned error: %v", err)
	}

	out := r.Stdout.String()
	for _, want := range []string{
		"#1 octo Roadmap", "Summary", "Total items", "Field completeness",
		"Status", "66.7%", "Field values", "Done", `Grouped by "Status"`,
		"Repositories", "octo/api", "Views", "BOARD",
		"Lead time (created to closed)", "3.0d",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q\n%s", want, out)
		}
	}
}

func TestRenderProjectStatsOmitsEmptySections(t *testing.T) {
	r := ghrender.NewStringRenderer(nil)
	report := &ProjectStatsReport{Label: "#1 octo", Title: "Empty"}

	if err := RenderProjectStats(&r.Renderer, report); err != nil {
		t.Fatalf("RenderProjectStats returned error: %v", err)
	}

	out := r.Stdout.String()
	for _, unwanted := range []string{"Field completeness", "Field values", "Grouped by", "Lead time"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output should not contain %q\n%s", unwanted, out)
		}
	}
}
