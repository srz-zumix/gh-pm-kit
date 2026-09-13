package render

import (
	"bytes"
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

// TestProjectLintItemLabelTruncationAcrossRenderers guards the no-truncation path used by the
// Actions and Markdown renderers: the terminal table truncates long titles (maxTitle 40) while the
// annotation and Markdown outputs must retain the full title (maxTitle 0). All other fixtures use
// short titles, so a regression to unconditional truncation would otherwise go unnoticed.
func TestProjectLintItemLabelTruncationAcrossRenderers(t *testing.T) {
	longTitle := "This project item title is intentionally longer than forty characters"
	report := &ProjectLintReport{
		Label: "#1 octo",
		Title: "Roadmap",
		Findings: []ProjectLintFinding{
			{RuleID: "PM010", RuleName: "closed-issue-not-done", Severity: "error", Message: "closed but in progress", ItemID: "i7", ItemTitle: longTitle, ItemNumber: 7},
		},
		Summary: ProjectLintSummary{Errors: 1},
	}

	wantTruncated := "#7 " + string([]rune(longTitle)[:37]) + "..."

	r := ghrender.NewStringRenderer(nil)
	if err := RenderProjectLint(&r.Renderer, report); err != nil {
		t.Fatalf("RenderProjectLint returned error: %v", err)
	}
	terminal := r.Stdout.String()
	if !strings.Contains(terminal, wantTruncated) {
		t.Errorf("terminal output should contain the truncated label %q\n%s", wantTruncated, terminal)
	}
	if strings.Contains(terminal, longTitle) {
		t.Errorf("terminal output should not contain the full title\n%s", terminal)
	}

	var annotations bytes.Buffer
	if err := RenderProjectLintAnnotations(&annotations, report); err != nil {
		t.Fatalf("RenderProjectLintAnnotations returned error: %v", err)
	}
	if !strings.Contains(annotations.String(), "#7 "+longTitle) {
		t.Errorf("annotation output should retain the full title\n%s", annotations.String())
	}

	var markdown bytes.Buffer
	if err := RenderProjectLintMarkdown(&markdown, report); err != nil {
		t.Fatalf("RenderProjectLintMarkdown returned error: %v", err)
	}
	if !strings.Contains(markdown.String(), "#7 "+longTitle) {
		t.Errorf("Markdown output should retain the full title\n%s", markdown.String())
	}
}
