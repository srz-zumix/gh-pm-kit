// Package lint implements rule-based auditing of a GitHub Project v2.
package lint

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// Severity classifies how serious a finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Severities lists the severity values accepted by flags and by the config file.
var Severities = []string{string(SeverityError), string(SeverityWarning), string(SeverityInfo)}

var severityRank = map[Severity]int{SeverityInfo: 1, SeverityWarning: 2, SeverityError: 3}

// AtLeast reports whether s is as serious as threshold.
func (s Severity) AtLeast(threshold Severity) bool {
	return severityRank[s] >= severityRank[threshold]
}

// ParseSeverity validates a severity string.
func ParseSeverity(s string) (Severity, error) {
	severity := Severity(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := severityRank[severity]; !ok {
		return "", fmt.Errorf("invalid severity %q: must be one of %s", s, strings.Join(Severities, ", "))
	}
	return severity, nil
}

// Options configures a lint run. The YAML tags describe the `lint:` section of
// the pm-kit config file; every option can also be set through a flag.
type Options struct {
	Rules              []string            `yaml:"rules"`
	Ignore             []string            `yaml:"ignore"`
	Severity           map[string]Severity `yaml:"severity"`
	StatusField        string              `yaml:"status-field"`
	DoneStatuses       []string            `yaml:"done-statuses"`
	InProgressStatuses []string            `yaml:"in-progress-statuses"`
	RequiredFields     []string            `yaml:"required-fields"`
	StaleDays          int                 `yaml:"stale-days"`
	StatusUpdateDays   int                 `yaml:"status-update-days"`
	IncludeArchived    bool                `yaml:"include-archived"`
	FailOn             Severity            `yaml:"fail-on"`
}

// Default option values used when neither a flag nor the config file sets them.
const (
	DefaultStatusField      = "Status"
	DefaultStaleDays        = 30
	DefaultStatusUpdateDays = 14
	DefaultFailOn           = SeverityError
)

// DefaultDoneStatuses and DefaultInProgressStatuses are matched case-insensitively.
var (
	DefaultDoneStatuses       = []string{"Done", "Closed", "Complete", "Completed"}
	DefaultInProgressStatuses = []string{"In Progress", "In Review", "Doing"}
)

// applyDefaults fills in unset options.
func (o *Options) applyDefaults() {
	if o.StatusField == "" {
		o.StatusField = DefaultStatusField
	}
	if len(o.DoneStatuses) == 0 {
		o.DoneStatuses = DefaultDoneStatuses
	}
	if len(o.InProgressStatuses) == 0 {
		o.InProgressStatuses = DefaultInProgressStatuses
	}
	if o.StaleDays == 0 {
		o.StaleDays = DefaultStaleDays
	}
	if o.StatusUpdateDays == 0 {
		o.StatusUpdateDays = DefaultStatusUpdateDays
	}
	if o.FailOn == "" {
		o.FailOn = DefaultFailOn
	}
}

// Context carries the collected project and the resolved options to the rules.
type Context struct {
	Project *projects.CollectedProject
	// Items excludes archived items unless Options.IncludeArchived is set.
	// Rules that inspect archived items read Project.Items instead.
	Items   []gh.ProjectV2Item
	Options Options

	now         time.Time
	statusField *gh.ProjectV2Field
}

// StatusOf returns the configured status field value of an item, or an empty
// string when the field is unset or does not exist in the project.
func (c *Context) StatusOf(item gh.ProjectV2Item) string {
	if c.statusField == nil {
		return ""
	}
	values := projects.ItemFieldValues(item, c.statusField.Name)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// IsDoneStatus reports whether a status value means the work is finished.
func (c *Context) IsDoneStatus(status string) bool {
	return containsFold(c.Options.DoneStatuses, status)
}

// IsInProgressStatus reports whether a status value means the work is ongoing.
func (c *Context) IsInProgressStatus(status string) bool {
	return containsFold(c.Options.InProgressStatuses, status)
}

// IsDone reports whether an item is finished, preferring the status field and
// falling back to the issue or pull request state.
func (c *Context) IsDone(item gh.ProjectV2Item) bool {
	if status := c.StatusOf(item); status != "" {
		return c.IsDoneStatus(status)
	}
	return isClosedState(item.Content.State)
}

func isClosedState(state string) bool {
	return state == "CLOSED" || state == "MERGED"
}

func containsFold(list []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), value) {
			return true
		}
	}
	return false
}

// reporter accumulates the findings produced by a single rule.
type reporter struct {
	rule     *ruleDef
	severity Severity
	findings []render.ProjectLintFinding
}

func (r *reporter) item(item gh.ProjectV2Item, format string, args ...any) {
	r.findings = append(r.findings, render.ProjectLintFinding{
		RuleID:     r.rule.ID,
		RuleName:   r.rule.Name,
		Severity:   string(r.severity),
		Message:    fmt.Sprintf(format, args...),
		ItemID:     item.ID,
		ItemTitle:  item.Content.Title,
		ItemURL:    item.Content.URL,
		ItemNumber: item.Content.Number,
	})
}

func (r *reporter) project(format string, args ...any) {
	r.findings = append(r.findings, render.ProjectLintFinding{
		RuleID:   r.rule.ID,
		RuleName: r.rule.Name,
		Severity: string(r.severity),
		Message:  fmt.Sprintf(format, args...),
	})
}

// ruleDef describes a single lint rule.
type ruleDef struct {
	ID              string
	Name            string
	Description     string
	DefaultSeverity Severity
	Check           func(c *Context, r *reporter)
}

// RuleInfo describes a rule for help output and flag completion.
type RuleInfo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	DefaultSeverity Severity `json:"defaultSeverity"`
}

// Rules returns the metadata of every registered rule, ordered by rule ID.
func Rules() []RuleInfo {
	infos := make([]RuleInfo, 0, len(rules))
	for _, r := range rules {
		infos = append(infos, RuleInfo{ID: r.ID, Name: r.Name, Description: r.Description, DefaultSeverity: r.DefaultSeverity})
	}
	return infos
}

// RuleIDs returns every registered rule ID, ordered by rule ID.
func RuleIDs() []string {
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.ID)
	}
	return ids
}

func findRule(id string) *ruleDef {
	for _, r := range rules {
		if strings.EqualFold(r.ID, id) {
			return r
		}
	}
	return nil
}

// Run evaluates the enabled rules against a collected project.
func Run(project *projects.CollectedProject, opts Options) (*render.ProjectLintReport, error) {
	opts.applyDefaults()

	enabled, err := enabledRules(opts)
	if err != nil {
		return nil, err
	}
	severities, err := resolveSeverities(opts)
	if err != nil {
		return nil, err
	}
	if _, err := ParseSeverity(string(opts.FailOn)); err != nil {
		return nil, err
	}

	ctx := newContext(project, opts)

	report := &render.ProjectLintReport{
		Label: project.Label(),
		Title: project.Project.Title,
	}
	for _, rule := range enabled {
		r := &reporter{rule: rule, severity: severities[rule.ID]}
		rule.Check(ctx, r)
		report.Findings = append(report.Findings, r.findings...)
	}
	for _, f := range report.Findings {
		switch Severity(f.Severity) {
		case SeverityError:
			report.Summary.Errors++
		case SeverityWarning:
			report.Summary.Warnings++
		default:
			report.Summary.Infos++
		}
	}
	return report, nil
}

// ShouldFail reports whether a report reaches the configured failure threshold.
func ShouldFail(report *render.ProjectLintReport, failOn Severity) bool {
	for _, f := range report.Findings {
		if Severity(f.Severity).AtLeast(failOn) {
			return true
		}
	}
	return false
}

func newContext(project *projects.CollectedProject, opts Options) *Context {
	items := make([]gh.ProjectV2Item, 0, len(project.Items))
	for _, item := range project.Items {
		if item.IsArchived && !opts.IncludeArchived {
			continue
		}
		items = append(items, item)
	}
	return &Context{
		Project:     project,
		Items:       items,
		Options:     opts,
		now:         time.Now(),
		statusField: project.FindField(opts.StatusField),
	}
}

func enabledRules(opts Options) ([]*ruleDef, error) {
	ignored := map[string]bool{}
	for _, id := range opts.Ignore {
		rule := findRule(id)
		if rule == nil {
			return nil, fmt.Errorf("unknown lint rule %q", id)
		}
		ignored[rule.ID] = true
	}

	var selected []*ruleDef
	if len(opts.Rules) == 0 {
		selected = rules
	} else {
		seen := map[string]bool{}
		for _, id := range opts.Rules {
			rule := findRule(id)
			if rule == nil {
				return nil, fmt.Errorf("unknown lint rule %q", id)
			}
			if seen[rule.ID] {
				continue
			}
			seen[rule.ID] = true
			selected = append(selected, rule)
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	}

	enabled := make([]*ruleDef, 0, len(selected))
	for _, rule := range selected {
		if !ignored[rule.ID] {
			enabled = append(enabled, rule)
		}
	}
	return enabled, nil
}

func resolveSeverities(opts Options) (map[string]Severity, error) {
	severities := make(map[string]Severity, len(rules))
	for _, rule := range rules {
		severities[rule.ID] = rule.DefaultSeverity
	}
	for id, severity := range opts.Severity {
		rule := findRule(id)
		if rule == nil {
			return nil, fmt.Errorf("unknown lint rule %q in severity overrides", id)
		}
		parsed, err := ParseSeverity(string(severity))
		if err != nil {
			return nil, fmt.Errorf("invalid severity for rule %q: %w", rule.ID, err)
		}
		severities[rule.ID] = parsed
	}
	return severities, nil
}
