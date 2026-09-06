// Package status provides CLI commands for GitHub Project v2 status updates.
package status

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects status list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list <number|URL>",
		Short: "List status updates posted on a GitHub Project v2",
		Long: "List the status updates posted on a GitHub Project v2, newest first.\n\n" +
			"Each update shows its posted date, status (INACTIVE, ON_TRACK, AT_RISK,\n" +
			"OFF_TRACK, COMPLETE), start and target dates, creator, and the first line of\n" +
			"its body. Use --format json to retrieve full bodies.\n\n" +
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
			updates, err := gh.ListProjectV2StatusUpdates(ctx, client, repo.Owner, number)
			if err != nil {
				return fmt.Errorf("failed to list status updates for project #%d of '%s': %w", number, repo.Owner, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectV2StatusUpdates(updates)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
