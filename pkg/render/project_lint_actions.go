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
	fmt.Fprintf(&b, "## Project lint: %s %s\n\n", escapeMarkdownHeading(report.Label), escapeMarkdownHeading(report.Title))
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

// Backslash is listed first so literal backslashes in user-controlled titles are escaped
// without double-escaping the backslashes the other rules insert (NewReplacer does a single
// pass and never re-scans its own output). "[" and "]" are escaped so a title cannot break
// the surrounding link text; "\r\n" precedes "\r"/"\n" so CRLF collapses into one <br>.
var markdownCellEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"[", "\\[",
	"]", "\\]",
	"|", "\\|",
	"\r\n", "<br>",
	"\n", "<br>",
	"\r", "<br>",
)

func escapeMarkdownCell(s string) string {
	return markdownCellEscaper.Replace(s)
}

// markdownHeadingEscaper protects the single-line Markdown heading from user-controlled
// project metadata. CR/LF are collapsed to spaces so the title cannot break out of the
// heading line, and the inline CommonMark punctuation that could alter rendering or inject
// markup (emphasis, code, links, raw HTML, strikethrough, table pipes) is backslash-escaped.
// Backslash is listed first so literal backslashes are escaped without touching the
// backslashes the other rules insert (NewReplacer does a single pass over the input).
var markdownHeadingEscaper = strings.NewReplacer(
	"\r\n", " ",
	"\r", " ",
	"\n", " ",
	"\\", "\\\\",
	"`", "\\`",
	"*", "\\*",
	"_", "\\_",
	"[", "\\[",
	"]", "\\]",
	"<", "\\<",
	">", "\\>",
	"~", "\\~",
	"|", "\\|",
)

func escapeMarkdownHeading(s string) string {
	return markdownHeadingEscaper.Replace(s)
}
