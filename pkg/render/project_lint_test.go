package render

import (
	"strings"
	"testing"

	"github.com/fatih/color"

	ghrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

func TestRenderProjectLint(t *testing.T) {
	r := ghrender.NewStringRenderer(nil)
	report := &ProjectLintReport{
		Label: "#1 octo",
		Title: "Roadmap",
		Findings: []ProjectLintFinding{
			{RuleID: "PM010", RuleName: "closed-issue-not-done", Severity: "error", Message: "issue is closed but the status is \"In Progress\"", ItemID: "i2", ItemTitle: "Closed but working", ItemNumber: 2},
			{RuleID: "PM005", RuleName: "unused-select-option", Severity: "info", Message: "option \"Backlog\" is not used by any item"},
		},
		Summary: ProjectLintSummary{Errors: 1, Infos: 1},
	}

	if err := RenderProjectLint(&r.Renderer, report); err != nil {
		t.Fatalf("RenderProjectLint returned error: %v", err)
	}

	out := r.Stdout.String()
	for _, want := range []string{
		"#1 octo Roadmap", "ERROR", "PM010 closed-issue-not-done", "#2 Closed but working",
		"INFO", "(project)", "1 error(s), 0 warning(s), 1 info(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q\n%s", want, out)
		}
	}
}

func TestRenderProjectLintWithoutFindings(t *testing.T) {
	r := ghrender.NewStringRenderer(nil)

	if err := RenderProjectLint(&r.Renderer, &ProjectLintReport{Label: "#1 octo", Title: "Roadmap"}); err != nil {
		t.Fatalf("RenderProjectLint returned error: %v", err)
	}
	if !strings.Contains(r.Stdout.String(), "No findings.") {
		t.Errorf("output should report that nothing was found\n%s", r.Stdout.String())
	}
}

// TestProjectLintSeverityLabelNormalizesCase verifies that mixed-case severities from a
// decoded report still map to the correct color instead of the default branch.
func TestProjectLintSeverityLabelNormalizesCase(t *testing.T) {
	prev := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = prev }()

	cases := []struct {
		severity string
		wantCode string // ANSI SGR foreground code
	}{
		{"error", "\x1b[31m"},   // red
		{"ERROR", "\x1b[31m"},   // red, mixed case must normalize
		{"warning", "\x1b[33m"}, // yellow
		{"Warning", "\x1b[33m"}, // yellow, mixed case must normalize
		{"info", "\x1b[36m"},    // cyan (default branch)
	}
	for _, tc := range cases {
		got := projectLintSeverityLabel(tc.severity, true)
		if !strings.Contains(got, tc.wantCode) {
			t.Errorf("severity %q: expected color code %q in %q", tc.severity, tc.wantCode, got)
		}
		if !strings.Contains(got, strings.ToUpper(tc.severity)) {
			t.Errorf("severity %q: label should be uppercased, got %q", tc.severity, got)
		}
	}
}
