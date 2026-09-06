package projects

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	pkgrender "github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewStatsCmd creates the projects stats command.
func NewStatsCmd() *cobra.Command {
	var ownerFlag string
	var includeArchived bool
	var groupBy string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "stats <number|URL>",
		Short: "Show statistics for a GitHub Project v2",
		Long: "Show aggregated statistics for a GitHub Project v2.\n\n" +
			"The report covers item totals by type and state, custom field completeness,\n" +
			"select/iteration value distribution (including options no item uses),\n" +
			"repository, assignee and label distribution, view layouts, and an\n" +
			"approximated lead time based on the issue createdAt/closedAt timestamps.\n\n" +
			"Archived items are excluded unless --include-archived is given; the archived\n" +
			"count itself is always reported.\n\n" +
			"The project can be specified by its number or by its URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			report, err := projects.CalcProjectStats(collected, projects.StatsOptions{
				IncludeArchived: includeArchived,
				GroupBy:         groupBy,
			})
			if err != nil {
				return fmt.Errorf("failed to calculate statistics for project #%d of '%s': %w", number, repo.Owner, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			return pkgrender.RenderProjectStats(renderer, report)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.BoolVar(&includeArchived, "include-archived", false, "Include archived items in the statistics")
	f.StringVar(&groupBy, "group-by", "", "Custom field name to break items down by")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
