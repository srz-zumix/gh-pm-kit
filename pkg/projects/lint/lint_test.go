package lint

import (
	"testing"
	"time"

	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

func statusValue(name string) gh.ProjectV2FieldValue {
	return gh.ProjectV2FieldValue{FieldName: "Status", ValueType: "SINGLE_SELECT", SelectName: name}
}

func iterationValue(title string) gh.ProjectV2FieldValue {
	return gh.ProjectV2FieldValue{FieldName: "Sprint", ValueType: "ITERATION", IterationTitle: title}
}

func daysAgo(days int) string {
	return time.Now().AddDate(0, 0, -days).Format(time.RFC3339)
}

func lintTestProject() *projects.CollectedProject {
	return &projects.CollectedProject{
		Host:    "github.com",
		Owner:   "octo",
		Number:  1,
		Project: &gh.ProjectV2{Number: 1, Title: "Roadmap"},
		Fields: []gh.ProjectV2Field{
			{
				Name:     "Status",
				DataType: "SINGLE_SELECT",
				Options: []gh.ProjectV2SelectOption{
					{Name: "Todo"}, {Name: "In Progress"}, {Name: "Done"}, {Name: "Backlog"},
				},
			},
			{
				Name:                "Sprint",
				DataType:            "ITERATION",
				Iterations:          []gh.ProjectV2IterationOption{{Title: "Sprint 2", StartDate: "2026-02-01"}},
				CompletedIterations: []gh.ProjectV2IterationOption{{Title: "Sprint 1", StartDate: "2026-01-01"}},
			},
		},
		Views: []gh.ProjectV2View{
			{Name: "Board", Filter: "status:Todo -label:bug is:open"},
			{Name: "Broken", Filter: "priorety:High"},
		},
		StatusUpdates: []gh.ProjectV2StatusUpdate{
			{Status: "ON_TRACK", CreatedAt: daysAgo(40)},
		},
		Items: []gh.ProjectV2Item{
			{
				ID:          "i1",
				Content:     gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeIssue, Number: 1, Title: "Healthy", State: "OPEN", Assignees: []string{"alice"}, UpdatedAt: daysAgo(1)},
				FieldValues: []gh.ProjectV2FieldValue{statusValue("Todo")},
			},
			{
				ID:          "i2",
				Content:     gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeIssue, Number: 2, Title: "Closed but working", State: "CLOSED", Assignees: []string{"bob"}},
				FieldValues: []gh.ProjectV2FieldValue{statusValue("In Progress")},
			},
			{
				ID:          "i3",
				Content:     gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeIssue, Number: 3, Title: "Done but open", State: "OPEN", Assignees: []string{"bob"}},
				FieldValues: []gh.ProjectV2FieldValue{statusValue("Done")},
			},
			{
				ID:      "i4",
				Content: gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeDraftIssue, Title: "Spike"},
			},
			{
				ID:      "i5",
				Content: gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeDraftIssue, Title: "spike"},
			},
			{
				ID:          "i6",
				Content:     gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeIssue, Number: 6, Title: "Forgotten", State: "OPEN", UpdatedAt: daysAgo(60)},
				FieldValues: []gh.ProjectV2FieldValue{statusValue("In Progress")},
			},
			{
				ID:          "i7",
				IsArchived:  true,
				Content:     gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeIssue, Number: 7, Title: "Archived", State: "OPEN"},
				FieldValues: []gh.ProjectV2FieldValue{statusValue("Todo")},
			},
			{
				ID:      "i8",
				Content: gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeRedacted, Title: "Redacted"},
			},
			{
				ID:          "i9",
				Content:     gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeIssue, Number: 9, Title: "Late sprint", State: "OPEN", Assignees: []string{"alice"}},
				FieldValues: []gh.ProjectV2FieldValue{statusValue("Todo"), iterationValue("Sprint 1")},
			},
		},
	}
}

func runRule(t *testing.T, ruleID string, opts Options) []render.ProjectLintFinding {
	t.Helper()
	opts.Rules = []string{ruleID}
	report, err := Run(lintTestProject(), opts)
	if err != nil {
		t.Fatalf("Run(%s) returned error: %v", ruleID, err)
	}
	return report.Findings
}

func findingItemIDs(findings []render.ProjectLintFinding) []string {
	ids := make([]string, 0, len(findings))
	for _, f := range findings {
		ids = append(ids, f.ItemID)
	}
	return ids
}

func assertItemIDs(t *testing.T, ruleID string, findings []render.ProjectLintFinding, want ...string) {
	t.Helper()
	got := findingItemIDs(findings)
	if len(got) != len(want) {
		t.Fatalf("%s reported items %v, want %v", ruleID, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s finding %d is item %q, want %q", ruleID, i, got[i], want[i])
		}
	}
}

func TestRulesReportExpectedItems(t *testing.T) {
	cases := []struct {
		ruleID string
		opts   Options
		want   []string
	}{
		{ruleID: "PM001", want: []string{"i4", "i5", "i8"}},
		{ruleID: "PM003", want: []string{"i4", "i5"}},
		{ruleID: "PM007", want: []string{"i5"}},
		{ruleID: "PM010", want: []string{"i2"}},
		{ruleID: "PM011", want: []string{"i3"}},
		{ruleID: "PM012", want: []string{"i6"}},
		{ruleID: "PM013", want: []string{"i6"}},
		{ruleID: "PM014", want: []string{"i7"}},
		{ruleID: "PM015", want: []string{"i9"}},
		{ruleID: "PM016", want: []string{"i8"}},
	}
	for _, c := range cases {
		t.Run(c.ruleID, func(t *testing.T) {
			assertItemIDs(t, c.ruleID, runRule(t, c.ruleID, c.opts), c.want...)
		})
	}
}

func TestProjectWideRules(t *testing.T) {
	cases := []struct {
		ruleID      string
		wantCount   int
		wantMessage string
	}{
		{ruleID: "PM004", wantCount: 1, wantMessage: "the latest status update is 40 days old (allowed: 14)"},
		{ruleID: "PM005", wantCount: 1, wantMessage: `option "Backlog" of field "Status" is not used by any item`},
		{ruleID: "PM006", wantCount: 1, wantMessage: `view "Broken" filters on unknown qualifier "priorety"`},
	}
	for _, c := range cases {
		t.Run(c.ruleID, func(t *testing.T) {
			findings := runRule(t, c.ruleID, Options{})
			if len(findings) != c.wantCount {
				t.Fatalf("%s reported %d findings, want %d: %+v", c.ruleID, len(findings), c.wantCount, findings)
			}
			if findings[0].ItemID != "" {
				t.Errorf("%s should report a project-wide finding, got item %q", c.ruleID, findings[0].ItemID)
			}
			if findings[0].Message != c.wantMessage {
				t.Errorf("%s message = %q, want %q", c.ruleID, findings[0].Message, c.wantMessage)
			}
		})
	}
}

func TestPM002RequiredFields(t *testing.T) {
	findings := runRule(t, "PM002", Options{RequiredFields: []string{"Sprint", "Priority"}})

	sprintFindings := 0
	missingFieldFindings := 0
	for _, f := range findings {
		if f.ItemID == "" {
			missingFieldFindings++
			continue
		}
		sprintFindings++
	}
	if missingFieldFindings != 1 {
		t.Errorf("PM002 reported %d project findings, want 1 for the unknown field", missingFieldFindings)
	}
	// Only i9 has a Sprint value, so the remaining seven non-archived items are reported.
	if sprintFindings != 7 {
		t.Errorf("PM002 reported %d item findings, want 7", sprintFindings)
	}
}

func TestIncludeArchivedExtendsScope(t *testing.T) {
	opts := Options{DoneStatuses: []string{"Todo"}}
	if got := findingItemIDs(runRule(t, "PM011", opts)); len(got) != 2 {
		t.Fatalf("PM011 reported %v, want the two non-archived Todo items", got)
	}

	opts.IncludeArchived = true
	got := findingItemIDs(runRule(t, "PM011", opts))
	if len(got) != 3 || got[1] != "i7" {
		t.Fatalf("PM011 reported %v, want the archived item i7 to be included", got)
	}
}

func TestCustomStatusFieldIsRequiredForStatusRules(t *testing.T) {
	findings := runRule(t, "PM001", Options{StatusField: "Stage"})
	if len(findings) != 0 {
		t.Errorf("PM001 should report nothing when the status field does not exist, got %+v", findings)
	}
}

func TestRunSeverityOverrideAndSummary(t *testing.T) {
	report, err := Run(lintTestProject(), Options{
		Rules:    []string{"PM003"},
		Severity: map[string]Severity{"PM003": SeverityError},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if report.Summary.Errors != 2 || report.Summary.Infos != 0 {
		t.Errorf("summary = %+v, want 2 errors and 0 infos", report.Summary)
	}
	if report.Label != "#1 octo" || report.Title != "Roadmap" {
		t.Errorf("label/title = %q/%q, want #1 octo/Roadmap", report.Label, report.Title)
	}
}

func TestRunIgnoreSkipsRules(t *testing.T) {
	report, err := Run(lintTestProject(), Options{Rules: []string{"PM003", "PM007"}, Ignore: []string{"PM003"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, f := range report.Findings {
		if f.RuleID == "PM003" {
			t.Fatalf("PM003 should have been ignored, got %+v", f)
		}
	}
}

func TestRunRejectsUnknownRuleAndSeverity(t *testing.T) {
	if _, err := Run(lintTestProject(), Options{Rules: []string{"PM999"}}); err == nil {
		t.Error("Run should fail for an unknown rule in --rule")
	}
	if _, err := Run(lintTestProject(), Options{Ignore: []string{"PM999"}}); err == nil {
		t.Error("Run should fail for an unknown rule in --ignore")
	}
	if _, err := Run(lintTestProject(), Options{Severity: map[string]Severity{"PM001": "fatal"}}); err == nil {
		t.Error("Run should fail for an invalid severity override")
	}
	if _, err := Run(lintTestProject(), Options{FailOn: "fatal"}); err == nil {
		t.Error("Run should fail for an invalid fail-on severity")
	}
}

func TestShouldFailThreshold(t *testing.T) {
	report := &render.ProjectLintReport{
		Findings: []render.ProjectLintFinding{{Severity: string(SeverityWarning)}},
	}
	if ShouldFail(report, SeverityError) {
		t.Error("a warning should not fail a run with --fail-on error")
	}
	if !ShouldFail(report, SeverityWarning) {
		t.Error("a warning should fail a run with --fail-on warning")
	}
	if ShouldFail(&render.ProjectLintReport{}, SeverityInfo) {
		t.Error("a report without findings should never fail")
	}
}
