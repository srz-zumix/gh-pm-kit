// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List GitHub Projects v2",
		Long: "List GitHub Projects v2 for an owner.\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := parser.Repository(parser.RepositoryOwnerWithHost(ownerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve owner: %w", err)
			}

			client, err := gh.NewGitHubClientWithRepo(repo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}

			ctx := cmd.Context()
			projects, err := gh.ListProjectsV2(ctx, client, repo.Owner)
			if err != nil {
				return fmt.Errorf("failed to list projects for '%s': %w", repo.Owner, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectsV2(projects, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
