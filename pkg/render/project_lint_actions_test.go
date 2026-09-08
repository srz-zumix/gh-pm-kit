package render

import (
	"bytes"
	"strings"
	"testing"
)

func lintActionsReport() *ProjectLintReport {
	return &ProjectLintReport{
		Label: "#1 octo",
		Title: "Roadmap",
		Findings: []ProjectLintFinding{
			{RuleID: "PM010", RuleName: "closed-issue-not-done", Severity: "error", Message: "issue is closed but the status is \"In Progress\"", ItemID: "i2", ItemTitle: "Closed | working", ItemNumber: 2, ItemURL: "https://github.com/octo/repo/issues/2"},
			{RuleID: "PM012", RuleName: "stale-item", Severity: "warning", Message: "not updated for 60 days", ItemID: "i6", ItemTitle: "Stale", ItemNumber: 6},
			{RuleID: "PM005", RuleName: "unused-select-option", Severity: "info", Message: "option \"Backlog\" is not used by any item"},
		},
		Summary: ProjectLintSummary{Errors: 1, Warnings: 1, Infos: 1},
	}
}

func TestRenderProjectLintAnnotations(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderProjectLintAnnotations(&buf, lintActionsReport()); err != nil {
		t.Fatalf("RenderProjectLintAnnotations returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	want := []string{
		"::error title=PM010 closed-issue-not-done::#2 Closed | working: issue is closed but the status is \"In Progress\" (https://github.com/octo/repo/issues/2)",
		"::warning title=PM012 stale-item::#6 Stale: not updated for 60 days",
		"::notice title=PM005 unused-select-option::option \"Backlog\" is not used by any item",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d annotations, want %d\n%s", len(lines), len(want), buf.String())
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("annotation %d:\n got: %s\nwant: %s", i, lines[i], w)
		}
	}
}

func TestRenderProjectLintAnnotationsEscapesSpecialCharacters(t *testing.T) {
	var buf bytes.Buffer
	report := &ProjectLintReport{Findings: []ProjectLintFinding{
		{RuleID: "PM001", RuleName: "no-status", Severity: "warning", Message: "50% done\nsecond line"},
	}}
	if err := RenderProjectLintAnnotations(&buf, report); err != nil {
		t.Fatalf("RenderProjectLintAnnotations returned error: %v", err)
	}
	if got, want := strings.TrimSuffix(buf.String(), "\n"), "::warning title=PM001 no-status::50%25 done%0Asecond line"; got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestRenderProjectLintMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderProjectLintMarkdown(&buf, lintActionsReport()); err != nil {
		t.Fatalf("RenderProjectLintMarkdown returned error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"## Project lint: #1 octo Roadmap",
		"1 error(s), 1 warning(s), 1 info(s)",
		"| Severity | Rule | Item | Message |",
		"| ERROR | PM010 closed-issue-not-done | [#2 Closed \\| working](https://github.com/octo/repo/issues/2) |",
		"| WARNING | PM012 stale-item | #6 Stale | not updated for 60 days |",
		"| INFO | PM005 unused-select-option | (project) |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q\n%s", want, out)
		}
	}
}

func TestRenderProjectLintMarkdownWithoutFindings(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderProjectLintMarkdown(&buf, &ProjectLintReport{Label: "#1 octo", Title: "Roadmap"}); err != nil {
		t.Fatalf("RenderProjectLintMarkdown returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "No findings.") {
		t.Errorf("output should report that nothing was found\n%s", buf.String())
	}
}

// TestRenderProjectLintMarkdownEscapesBrackets verifies that user-controlled titles with
// brackets, backslashes, pipes, and newlines are escaped in both plain cells and link labels
// so the job-summary table and its links stay valid.
func TestRenderProjectLintMarkdownEscapesBrackets(t *testing.T) {
	var buf bytes.Buffer
	report := &ProjectLintReport{
		Label: "#1 octo",
		Title: "Roadmap",
		Findings: []ProjectLintFinding{
			// Linked item: the title becomes the link label.
			{RuleID: "PM010", RuleName: "closed-issue-not-done", Severity: "error", Message: `bug in a\[b]`, ItemID: "i2", ItemTitle: `[WIP] a|b`, ItemNumber: 2, ItemURL: "https://github.com/octo/repo/issues/2"},
			// Unlinked (project-wide) finding: message goes into a plain cell.
			{RuleID: "PM005", RuleName: "unused-select-option", Severity: "info", Message: "line1\r\nline2"},
		},
		Summary: ProjectLintSummary{Errors: 1, Infos: 1},
	}
	if err := RenderProjectLintMarkdown(&buf, report); err != nil {
		t.Fatalf("RenderProjectLintMarkdown returned error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		// Link label: brackets and pipe escaped, backslash-before-bracket not reprocessed.
		`[#2 \[WIP\] a\|b](https://github.com/octo/repo/issues/2)`,
		// Plain message cell: literal backslash then escaped brackets.
		`bug in a\\\[b\]`,
		// CRLF collapses into a single <br>.
		"line1<br>line2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "line1<br><br>line2") {
		t.Errorf("CRLF should collapse into a single <br>\n%s", out)
	}
}
