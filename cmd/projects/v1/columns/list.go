// Package v1columns provides the CLI command for listing GitHub Project (classic) columns.
package v1columns

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects v1 columns list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	var repoFlag string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list <number|URL>",
		Short: "List columns of a GitHub Project (classic)",
		Long: "List columns of a GitHub Project (classic).\n\n" +
			"The project can be specified by its number or by its URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').\n" +
			"Repo format: '[HOST/]OWNER/REPO' (e.g. 'my-org/my-repo').",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := parser.GetProjectNumberFromString(args[0])
			if err != nil {
				return fmt.Errorf("invalid project number or URL %q: %w", args[0], err)
			}

			// Prefer owner from URL if provided.
			if projectURL, _ := parser.ParseProjectURL(args[0]); projectURL != nil {
				if ownerFlag == "" {
					ownerFlag = projectURL.Host + "/" + projectURL.Owner
				}
			}

			var owner, repoName string
			if repoFlag != "" {
				repo, err := parser.Repository(parser.RepositoryInput(repoFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve repository: %w", err)
				}
				owner = repo.Owner
				repoName = repo.Name
			} else {
				repo, err := parser.Repository(parser.RepositoryOwnerWithHost(ownerFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve owner: %w", err)
				}
				owner = repo.Owner
			}

			clientRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(ownerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve owner for client: %w", err)
			}
			client, err := gh.NewGitHubClientWithRepo(clientRepo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}

			ctx := cmd.Context()
			project, err := gh.GetProjectV1ByNumber(ctx, client, repository.Repository{Owner: owner, Name: repoName}, number)
			if err != nil {
				return fmt.Errorf("failed to get classic project #%d for '%s': %w", number, owner, err)
			}

			columns, err := client.ListProjectV1Columns(ctx, project.ID)
			if err != nil {
				return fmt.Errorf("failed to list columns for classic project #%d of '%s': %w", number, owner, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectV1Columns(columns, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVarP(&repoFlag, "repo", "R", "", "Repository in the format '[HOST/]OWNER/REPO'; for repository-scoped projects")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
