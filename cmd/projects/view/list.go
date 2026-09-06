// Package view provides CLI commands for GitHub Project v2 views.
package view

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects view list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	var fields []string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list <number|URL>",
		Short: "List views in a GitHub Project v2",
		Long: "List the views configured in a GitHub Project v2, including their layout,\n" +
			"filter, grouping and sort criteria. Useful for auditing how a project is\n" +
			"presented to its users.\n\n" +
			"The project can be specified by its number or by its URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, number, err := projects.ResolveProject(args[0], ownerFlag)
			if err != nil {
				return err
			}

			client, err := gh.NewGitHubClientWithRepo(repo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}

			ctx := cmd.Context()
			views, err := gh.ListProjectV2Views(ctx, client, repo.Owner, number)
			if err != nil {
				return fmt.Errorf("failed to list views for project #%d of '%s': %w", number, repo.Owner, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectV2Views(views, fields)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	cmdutil.StringSliceEnumFlag(cmd, &fields, "field", "", nil, render.ProjectV2ViewFields, "Fields to display (default: NUMBER,NAME,LAYOUT,FILTER,GROUPBY,SORTBY)")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
