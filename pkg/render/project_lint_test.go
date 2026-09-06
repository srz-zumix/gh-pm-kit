package render

import (
	"strings"
	"testing"

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
