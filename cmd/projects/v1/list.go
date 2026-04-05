// Package v1 provides CLI commands for GitHub Projects (classic) v1 management.
package v1

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects v1 list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	var repoFlag string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List GitHub Projects (classic)",
		Long: "List GitHub Projects (classic) for an owner or repository.\n\n" +
			"If --repo is specified, repository projects are listed.\n" +
			"If --owner is specified, projects for that organization or user are listed.\n" +
			"If neither is specified, the current repository's owner projects are listed.\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').\n" +
			"Repo format: '[HOST/]OWNER/REPO' (e.g. 'my-org/my-repo').",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoFlag != "" {
				repo, err := parser.Repository(parser.RepositoryInput(repoFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve repository: %w", err)
				}
				client, err := gh.NewGitHubClientWithRepo(repo)
				if err != nil {
					return fmt.Errorf("failed to create GitHub client: %w", err)
				}
				ctx := cmd.Context()
				projects, err := gh.ListProjectsV1(ctx, client, repo)
				if err != nil {
					return fmt.Errorf("failed to list classic projects for '%s/%s': %w", repo.Owner, repo.Name, err)
				}
				renderer := render.NewRenderer(opts.Exporter)
				return renderer.RenderProjectsV1(projects, nil)
			}

			repo, err := parser.Repository(parser.RepositoryOwnerWithHost(ownerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve owner: %w", err)
			}
			client, err := gh.NewGitHubClientWithRepo(repo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}
			ctx := cmd.Context()
			projects, err := gh.ListProjectsV1(ctx, client, repo)
			if err != nil {
				return fmt.Errorf("failed to list classic projects for '%s': %w", repo.Owner, err)
			}
			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectsV1(projects, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVarP(&repoFlag, "repo", "R", "", "Repository in the format '[HOST/]OWNER/REPO'; lists repository projects")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
