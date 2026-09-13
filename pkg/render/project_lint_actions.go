package render

import (
	"fmt"
	"io"
	"os"
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

// markdownInlinePunctuation lists the CommonMark/GFM inline markers that are backslash-escaped so
// user-controlled project text renders literally instead of as emphasis, code, links, raw HTML,
// strikethrough, or table pipes. Backslash is listed first so literal backslashes are escaped
// without touching the backslashes the later rules insert (strings.NewReplacer does a single pass
// over the input and never re-scans its own output). The cell and heading escapers share this list
// so they cannot drift apart; they only differ in how line breaks are folded.
var markdownInlinePunctuation = []string{
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
}

// markdownCellEscaper escapes inline punctuation and folds CR/LF into a single <br> so multi-line
// values stay inside one Markdown table cell ("\r\n" precedes "\r"/"\n" to collapse CRLF once).
var markdownCellEscaper = strings.NewReplacer(append(append([]string{}, markdownInlinePunctuation...),
	"\r\n", "<br>",
	"\n", "<br>",
	"\r", "<br>",
)...)

func escapeMarkdownCell(s string) string {
	return markdownCellEscaper.Replace(s)
}

// markdownHeadingEscaper escapes inline punctuation and folds CR/LF into spaces so user-controlled
// project metadata cannot break out of the single-line Markdown heading.
var markdownHeadingEscaper = strings.NewReplacer(append(append([]string{}, markdownInlinePunctuation...),
	"\r\n", " ",
	"\r", " ",
	"\n", " ",
)...)

func escapeMarkdownHeading(s string) string {
	return markdownHeadingEscaper.Replace(s)
}

// WriteProjectLintMarkdown appends the Markdown report to path, creating it if necessary, or writes
// it to out when path is "-". Keeping the file/stream handling in the render layer lets the command
// stay a thin orchestrator and lets the output behavior be reused and tested here.
func WriteProjectLintMarkdown(path string, out io.Writer, report *ProjectLintReport) (err error) {
	if path == "-" {
		return RenderProjectLintMarkdown(out, report)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	return RenderProjectLintMarkdown(file, report)
}
