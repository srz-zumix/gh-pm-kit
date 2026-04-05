// Package v1cards provides the CLI command for listing GitHub Project (classic) column cards.
package v1cards

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects v1 cards list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list (<column-id> | <project-url|number> <column-name>)",
		Short: "List cards in a GitHub Project (classic) column",
		Long: "List cards in a GitHub Project (classic) column.\n\n" +
			"Usage forms:\n" +
			"  list <column-id>                         — list by numeric column ID\n" +
			"  list <project-url|number> <column-name>  — list by project URL (or number) and column name\n\n" +
			"Column IDs can be obtained from the 'projects v1 columns list' command.\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').\n" +
			"Repo format: '[HOST/]OWNER/REPO' (e.g. 'my-org/my-repo').",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Two-argument form: <project-url|number> <column-name>
			if len(args) == 2 {
				projectArg := args[0]
				columnName := args[1]

				projectNumber, err := parser.GetProjectNumberFromString(projectArg)
				if err != nil {
					return fmt.Errorf("invalid project number or URL %q: %w", projectArg, err)
				}

				// Extract owner from URL if provided.
				if projectURL, _ := parser.ParseProjectURL(projectArg); projectURL != nil {
					if ownerFlag == "" {
						ownerFlag = projectURL.Host + "/" + projectURL.Owner
					}
				}

				clientRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(ownerFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve owner: %w", err)
				}
				client, err := gh.NewGitHubClientWithRepo(clientRepo)
				if err != nil {
					return fmt.Errorf("failed to create GitHub client: %w", err)
				}

				ctx := cmd.Context()
				project, err := gh.GetProjectV1ByNumber(ctx, client, repository.Repository{Owner: clientRepo.Owner}, projectNumber)
				if err != nil {
					return fmt.Errorf("failed to get classic project #%d for '%s': %w", projectNumber, clientRepo.Owner, err)
				}

				columns, err := client.ListProjectV1Columns(ctx, project.ID)
				if err != nil {
					return fmt.Errorf("failed to list columns for project #%d: %w", projectNumber, err)
				}

				var columnID int64
				for _, col := range columns {
					if strings.EqualFold(col.Name, columnName) {
						columnID = col.ID
						break
					}
				}
				if columnID == 0 {
					return fmt.Errorf("column %q not found in project #%d", columnName, projectNumber)
				}

				cards, err := client.ListProjectV1Cards(ctx, columnID)
				if err != nil {
					return fmt.Errorf("failed to list cards for column %q: %w", columnName, err)
				}

				renderer := render.NewRenderer(opts.Exporter)
				return renderer.RenderProjectV1Cards(cards, nil)
			}

			// Single-argument form: <column-id>
			columnID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || columnID <= 0 {
				return fmt.Errorf("invalid column ID %q: must be a positive integer", args[0])
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
			cards, err := client.ListProjectV1Cards(ctx, columnID)
			if err != nil {
				return fmt.Errorf("failed to list cards for column %d: %w", columnID, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectV1Cards(cards, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
