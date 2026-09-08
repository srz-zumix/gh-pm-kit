package render

import (
	"fmt"
	"io"
	"strings"
)

// projectLintAnnotationLevels maps a lint severity to a GitHub Actions annotation level.
var projectLintAnnotationLevels = map[string]string{
	"error":   "error",
	"warning": "warning",
	"info":    "notice",
}

// Workflow commands need percent, CR and LF encoded; property values also need ':' and ','.
var (
	annotationDataEscaper     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	annotationPropertyEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

// RenderProjectLintAnnotations writes one GitHub Actions workflow command per finding.
func RenderProjectLintAnnotations(w io.Writer, report *ProjectLintReport) error {
	for _, f := range report.Findings {
		level, ok := projectLintAnnotationLevels[strings.ToLower(f.Severity)]
		if !ok {
			level = "notice"
		}
		title := annotationPropertyEscaper.Replace(f.RuleID + " " + f.RuleName)
		message := annotationDataEscaper.Replace(projectLintAnnotationMessage(f))
		if _, err := fmt.Fprintf(w, "::%s title=%s::%s\n", level, title, message); err != nil {
			return err
		}
	}
	return nil
}

func projectLintAnnotationMessage(f ProjectLintFinding) string {
	message := f.Message
	if f.ItemID != "" {
		message = projectLintItemLabel(f, 0) + ": " + message
	}
	if f.ItemURL != "" {
		message += " (" + f.ItemURL + ")"
	}
	return message
}

// RenderProjectLintMarkdown writes a Markdown report suitable for GITHUB_STEP_SUMMARY.
func RenderProjectLintMarkdown(w io.Writer, report *ProjectLintReport) error {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project lint: %s %s\n\n", report.Label, report.Title)
	fmt.Fprintf(&b, "%s\n\n", projectLintSummaryLine(report.Summary))

	if len(report.Findings) == 0 {
		b.WriteString("No findings.\n")
	} else {
		b.WriteString("| Severity | Rule | Item | Message |\n")
		b.WriteString("| --- | --- | --- | --- |\n")
		for _, f := range report.Findings {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
				strings.ToUpper(f.Severity),
				escapeMarkdownCell(f.RuleID+" "+f.RuleName),
				projectLintMarkdownItem(f),
				escapeMarkdownCell(f.Message))
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func projectLintMarkdownItem(f ProjectLintFinding) string {
	label := escapeMarkdownCell(projectLintItemLabel(f, 0))
	if f.ItemURL == "" {
		return label
	}
	return "[" + label + "](" + f.ItemURL + ")"
}

func escapeMarkdownCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\r\n", "<br>", "\n", "<br>", "\r", "<br>").Replace(s)
}
