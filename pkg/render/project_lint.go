package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	ghrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

// ProjectLintFinding is a single rule violation reported by projects lint.
type ProjectLintFinding struct {
	RuleID    string `json:"ruleId"`
	RuleName  string `json:"ruleName"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	ItemID    string `json:"itemId,omitempty"`
	ItemTitle string `json:"itemTitle,omitempty"`
	ItemURL   string `json:"itemUrl,omitempty"`
	// ItemNumber is 0 for draft issues and for project-wide findings.
	ItemNumber int `json:"itemNumber,omitempty"`
}

// ProjectLintSummary holds the finding counts per severity.
type ProjectLintSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// ProjectLintReport is the top-level report for rendering lint results.
type ProjectLintReport struct {
	Label    string               `json:"label"`
	Title    string               `json:"title"`
	Findings []ProjectLintFinding `json:"findings"`
	Summary  ProjectLintSummary   `json:"summary"`
}

// RenderProjectLint renders a ProjectLintReport using the provided Renderer.
func RenderProjectLint(r *ghrender.Renderer, report *ProjectLintReport) error {
	if r.HasExporter() {
		return r.RenderExportedData(report)
	}

	r.WriteLine(fmt.Sprintf("%s %s", report.Label, report.Title))
	if len(report.Findings) == 0 {
		r.WriteLine("No findings.")
		return nil
	}

	table := r.NewTableWriter([]string{"SEVERITY", "RULE", "ITEM", "MESSAGE"})
	for _, f := range report.Findings {
		table.Append([]string{
			projectLintSeverityLabel(f.Severity, r.Color),
			f.RuleID + " " + f.RuleName,
			projectLintItemLabel(f),
			f.Message,
		})
	}
	if err := table.Render(); err != nil {
		return err
	}

	r.WriteLine(projectLintSummaryLine(report.Summary))
	return nil
}

func projectLintSeverityLabel(severity string, colorize bool) string {
	upper := strings.ToUpper(severity)
	if !colorize {
		return upper
	}
	// Match on the normalized severity so decoded reports using mixed case (e.g. "ERROR")
	// still map to the correct color instead of falling through to the default.
	switch strings.ToLower(severity) {
	case "error":
		return color.RedString(upper)
	case "warning":
		return color.YellowString(upper)
	default:
		return color.CyanString(upper)
	}
}

// projectLintItemLabel builds the ITEM column, marking project-wide findings.
func projectLintItemLabel(f ProjectLintFinding) string {
	if f.ItemID == "" {
		return "(project)"
	}
	title := truncate(f.ItemTitle, 40)
	if f.ItemNumber == 0 {
		return title
	}
	return "#" + strconv.Itoa(f.ItemNumber) + " " + title
}

func projectLintSummaryLine(s ProjectLintSummary) string {
	return fmt.Sprintf("%d error(s), %d warning(s), %d info(s)", s.Errors, s.Warnings, s.Infos)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
