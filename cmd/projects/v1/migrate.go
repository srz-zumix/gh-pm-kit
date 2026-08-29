// Package v1 provides CLI commands for GitHub Projects (classic) v1 management.
package v1

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
)

// NewMigrateCmd creates the projects v1 migrate command.
func NewMigrateCmd() *cobra.Command {
	var srcOwnerFlag string
	var srcRepoFlag string
	var dstOwnerFlag string
	var issueRepoFlag string
	var createIssue bool
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "migrate <number|URL>",
		Short: "Migrate a GitHub Project (classic) to GitHub Projects v2",
		Long: "Migrate a GitHub Project (classic) to a new GitHub Projects v2 project.\n\n" +
			"The source classic project is specified by its number or URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"A new Projects v2 project is created under the destination owner with the\n" +
			"source name, body, and open/closed state. Each column becomes an option in a\n" +
			"'Column' single-select field, a board view grouped by that field is created to\n" +
			"mirror the classic layout, and every card is migrated in its source order with\n" +
			"the Column field set. Archived cards are migrated and archived again.\n\n" +
			"Note cards keep their note as the title and body. Issue and pull-request cards\n" +
			"are resolved on the source host so that their title and body are reproduced.\n\n" +
			"Cards are migrated as draft issues by default. If --repo is specified, the\n" +
			"migration first searches for an existing issue in that repository that carries\n" +
			"the migration marker and links it to the project. If no matching issue is found\n" +
			"and --create-issue is set, a new issue is created; otherwise a draft issue is\n" +
			"used as a fallback.\n\n" +
			"Already-migrated items are identified by a hidden marker and skipped\n" +
			"unless --overwrite is specified.\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').\n" +
			"Repo format: '[HOST/]OWNER/REPO' (e.g. 'my-org/my-repo').",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcNumber, err := parser.GetProjectNumberFromString(args[0])
			if err != nil {
				return fmt.Errorf("invalid source project number or URL %q: %w", args[0], err)
			}

			// Prefer repo/owner from URL over flags.
			if projectURL, _ := parser.ParseProjectURL(args[0]); projectURL != nil {
				if projectURL.Repo.Name != "" {
					srcRepoFlag = projectURL.Repo.Host + "/" + projectURL.Repo.Owner + "/" + projectURL.Repo.Name
				} else {
					srcOwnerFlag = projectURL.Repo.Host + "/" + projectURL.Repo.Owner
				}
			}

			if dstOwnerFlag == "" {
				return fmt.Errorf("destination owner is required: use --dst")
			}
			dstRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(dstOwnerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve destination owner: %w", err)
			}

			var srcClientRepo repository.Repository
			if srcRepoFlag != "" {
				srcClientRepo, err = parser.Repository(parser.RepositoryInput(srcRepoFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve source repository for client: %w", err)
				}
			} else {
				srcClientRepo, err = parser.Repository(parser.RepositoryOwnerWithHost(srcOwnerFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve source owner for client: %w", err)
				}
			}
			srcClient, dstClient, err := gh.NewGitHubClientWith2Repos(srcClientRepo, dstRepo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub clients: %w", err)
			}

			migrateOpts := &projects.MigrateV1Options{
				Overwrite: overwrite,
			}
			if issueRepoFlag != "" {
				issueRepo, err := parser.Repository(parser.RepositoryInput(issueRepoFlag))
				if err != nil {
					return fmt.Errorf("invalid --repo value %q: %w", issueRepoFlag, err)
				}
				migrateOpts.IssueRepo = &issueRepo
				migrateOpts.CreateIssue = createIssue
			}
			ctx := cmd.Context()
			p, err := projects.MigrateProjectV1ToV2(ctx, srcClient, dstClient, srcClientRepo.Host, srcClientRepo.Owner, srcClientRepo.Name, dstRepo.Owner, srcNumber, migrateOpts)
			if err != nil {
				return fmt.Errorf("failed to migrate classic project #%d from '%s' to '%s': %w", srcNumber, srcClientRepo.Owner, dstRepo.Owner, err)
			}
			logger.Info("Migrated classic project", "srcNumber", srcNumber, "srcOwner", srcClientRepo.Owner, "dstNumber", p.Number, "dstOwner", dstRepo.Owner, "url", p.URL)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVarP(&srcOwnerFlag, "owner", "o", "", "Source owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVarP(&srcRepoFlag, "src-repo", "R", "", "Source repository in the format '[HOST/]OWNER/REPO'; for repository-scoped classic projects")
	f.StringVarP(&dstOwnerFlag, "dst", "d", "", "Destination owner in the format '[HOST/]OWNER' (required)")
	f.StringVarP(&issueRepoFlag, "repo", "r", "", "Repository in '[HOST/]OWNER/REPO' format; cards are linked to matching issues (by migration marker) in this repository")
	f.BoolVar(&createIssue, "create-issue", false, "When --repo is set and no matching issue is found, create a new issue instead of a draft issue")
	f.BoolVar(&overwrite, "overwrite", false, "Re-migrate already-migrated items instead of skipping them")
	return cmd
}
