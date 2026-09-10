package projects

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects/lint"
	pkgrender "github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewLintCmd creates the projects lint command.
func NewLintCmd() *cobra.Command {
	var ownerFlag string
	var configFlag string
	var colorFlag string
	var failOnFlag string
	var exitZero bool
	var annotate bool
	var summaryMarkdown string
	var opts lint.Options
	format := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "lint <number|URL>",
		Short: "Check a GitHub Project v2 against operational rules",
		Long: "Check a GitHub Project v2 against operational rules and report every violation.\n\n" +
			"Rules are selected with --rule and --ignore, and their severity can be changed\n" +
			"through the configuration file. Findings at or above --fail-on make the command\n" +
			"exit non-zero unless --exit-zero is given.\n\n" +
			"Defaults are read from '" + strings.Join(lint.ConfigPaths, "' or '") + "' when present;\n" +
			"use --config to load a different file. Flags always take precedence.\n\n" +
			"Rules:\n" + ruleHelp() + "\n" +
			"PM006 compares filter qualifiers with the project field names, so views that use\n" +
			"qualifiers this version does not know about are reported as findings.\n\n" +
			"Archived items are only checked when --include-archived is given, except for\n" +
			"PM014 which always inspects archived items.\n\n" +
			"Use --annotate on GitHub Actions to turn every finding into a workflow annotation,\n" +
			"and --summary-markdown \"$GITHUB_STEP_SUMMARY\" to append a Markdown report to the job summary.\n\n" +
			"The project can be specified by its number or by its URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := resolveLintOptions(cmd, configFlag, failOnFlag, opts)
			if err != nil {
				return err
			}

			repo, number, err := projects.ResolveProject(args[0], ownerFlag)
			if err != nil {
				return fmt.Errorf("failed to resolve project %q: %w", args[0], err)
			}

			client, err := gh.NewGitHubClientWithRepo(repo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}

			ctx := cmd.Context()
			collected, err := projects.CollectProject(ctx, client, repo, number)
			if err != nil {
				return fmt.Errorf("failed to collect project #%d of '%s': %w", number, repo.Owner, err)
			}

			report, err := lint.Run(collected, *resolved)
			if err != nil {
				return fmt.Errorf("failed to lint project #%d of '%s': %w", number, repo.Owner, err)
			}

			renderer := render.NewRenderer(format.Exporter)
			renderer.SetColor(colorFlag)
			if err := pkgrender.RenderProjectLint(renderer, report); err != nil {
				return fmt.Errorf("failed to render lint results: %w", err)
			}

			if annotate {
				// Keep the exported data stream clean by moving the workflow commands to stderr;
				// the Actions runner picks them up from either stream.
				if err := pkgrender.RenderProjectLintAnnotations(lintAuxiliaryOutput(renderer), report); err != nil {
					return fmt.Errorf("failed to write lint annotations: %w", err)
				}
			}

			if summaryMarkdown != "" {
				if err := writeLintMarkdown(summaryMarkdown, lintAuxiliaryOutput(renderer), report); err != nil {
					return fmt.Errorf("failed to write the Markdown summary to %q: %w", summaryMarkdown, err)
				}
			}

			if !exitZero && lint.ShouldFail(report, resolved.FailOn) {
				cmd.SilenceUsage = true
				return fmt.Errorf("lint found %d error(s), %d warning(s), and %d info(s) in project #%d of '%s'; failure threshold is %q",
					report.Summary.Errors, report.Summary.Warnings, report.Summary.Infos, number, repo.Owner, resolved.FailOn)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVar(&configFlag, "config", "", "Configuration file to read lint defaults from (default: "+strings.Join(lint.ConfigPaths, ", ")+")")
	f.BoolVar(&opts.IncludeArchived, "include-archived", false, "Check archived items as well")
	f.StringVar(&opts.StatusField, "status-field", lint.DefaultStatusField, "Name of the single-select field that holds the item status")
	f.StringSliceVar(&opts.DoneStatuses, "done-status", lint.DefaultDoneStatuses, "Status values that mean the work is finished")
	f.StringSliceVar(&opts.InProgressStatuses, "in-progress-status", lint.DefaultInProgressStatuses, "Status values that mean the work is ongoing")
	f.StringSliceVar(&opts.RequiredFields, "require", nil, "Field names that every item must have a value for (PM002)")
	f.IntVar(&opts.StaleDays, "stale-days", lint.DefaultStaleDays, "Days after which an unfinished item is reported as stale (PM012)")
	f.IntVar(&opts.StatusUpdateDays, "status-update-days", lint.DefaultStatusUpdateDays, "Days after which the latest status update is reported as stale (PM004)")
	f.BoolVar(&exitZero, "exit-zero", false, "Always exit with code 0, even when findings are reported")
	f.BoolVar(&annotate, "annotate", false, "Report every finding as a GitHub Actions workflow annotation")
	f.StringVar(&summaryMarkdown, "summary-markdown", "", "Append a Markdown report to the given file ('-' for stdout, or stderr while an export format is active), e.g. \"$GITHUB_STEP_SUMMARY\"")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Rules, "rule", "", nil, lint.RuleIDs(), "Rule IDs to run (default: all rules)")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Ignore, "ignore", "", nil, lint.RuleIDs(), "Rule IDs to skip")
	cmdutil.StringEnumFlag(cmd, &failOnFlag, "fail-on", "", string(lint.DefaultFailOn), lint.Severities, "Lowest severity that makes the command exit non-zero")
	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", render.ColorFlagAuto, render.ColorFlags, "Colorize output")
	cmdutil.AddFormatFlags(cmd, &format.Exporter)
	return cmd
}

// resolveLintOptions merges the configuration file with the flags the user set explicitly.
func resolveLintOptions(cmd *cobra.Command, configPath, failOn string, flagOpts lint.Options) (*lint.Options, error) {
	resolved := &lint.Options{}
	if configPath != "" {
		loaded, err := lint.LoadConfig(configPath)
		if err != nil {
			return nil, err
		}
		resolved = loaded
	} else {
		_, loaded, err := lint.FindConfig()
		if err != nil {
			return nil, err
		}
		if loaded != nil {
			resolved = loaded
		}
	}

	f := cmd.Flags()
	if f.Changed("include-archived") {
		resolved.IncludeArchived = flagOpts.IncludeArchived
	}
	if f.Changed("status-field") {
		resolved.StatusField = flagOpts.StatusField
	}
	if f.Changed("done-status") {
		resolved.DoneStatuses = flagOpts.DoneStatuses
	}
	if f.Changed("in-progress-status") {
		resolved.InProgressStatuses = flagOpts.InProgressStatuses
	}
	if f.Changed("require") {
		resolved.RequiredFields = flagOpts.RequiredFields
	}
	if f.Changed("stale-days") {
		resolved.StaleDays = flagOpts.StaleDays
	}
	if f.Changed("status-update-days") {
		resolved.StatusUpdateDays = flagOpts.StatusUpdateDays
	}
	if f.Changed("rule") {
		resolved.Rules = flagOpts.Rules
	}
	if f.Changed("ignore") {
		resolved.Ignore = flagOpts.Ignore
	}
	if f.Changed("fail-on") {
		severity, err := lint.ParseSeverity(failOn)
		if err != nil {
			return nil, err
		}
		resolved.FailOn = severity
	}
	// Default and canonicalize the merged fail-on threshold so the command uses the same
	// effective value that lint.Run applies internally. lint.Run only defaults its own copy,
	// so without this the caller-side FailOn (used by ShouldFail and the failure message)
	// could stay empty or keep a config-provided mixed-case value, both of which rank as 0
	// and would make any finding trigger a non-zero exit.
	if resolved.FailOn == "" {
		resolved.FailOn = lint.DefaultFailOn
	}
	failOnSeverity, err := lint.ParseSeverity(string(resolved.FailOn))
	if err != nil {
		return nil, err
	}
	resolved.FailOn = failOnSeverity
	return resolved, nil
}

// lintAuxiliaryOutput returns the stream for secondary output (Actions annotations and the
// Markdown summary written to '-'). When an exporter is active the primary stdout stream
// carries machine-readable data, so secondary output is routed to stderr to keep it valid.
func lintAuxiliaryOutput(renderer *render.Renderer) io.Writer {
	if renderer.HasExporter() {
		return renderer.IO.ErrOut
	}
	return renderer.IO.Out
}

// writeLintMarkdown appends the Markdown report to path, or writes it to out when path is "-".
func writeLintMarkdown(path string, out io.Writer, report *pkgrender.ProjectLintReport) (err error) {
	if path == "-" {
		return pkgrender.RenderProjectLintMarkdown(out, report)
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
	return pkgrender.RenderProjectLintMarkdown(file, report)
}

// ruleHelp renders the rule catalog for the command help.
func ruleHelp() string {
	var b strings.Builder
	for _, rule := range lint.Rules() {
		fmt.Fprintf(&b, "  %s %-26s %-8s %s\n", rule.ID, rule.Name, rule.DefaultSeverity, rule.Description)
	}
	return b.String()
}
