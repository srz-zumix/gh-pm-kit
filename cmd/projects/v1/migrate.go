// Package v1 provides CLI commands for GitHub Projects (classic) v1 management.
package v1

import (
	"fmt"

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
	var overwrite bool
	var prune bool

	cmd := &cobra.Command{
		Use:   "migrate <number|URL>",
		Short: "Migrate a GitHub Project (classic) to GitHub Projects v2",
		Long: "Migrate a GitHub Project (classic) to a new GitHub Projects v2 project.\n\n" +
			"The source classic project is specified by its number or URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"A new Projects v2 project is created under the destination owner.\n" +
			"Each column becomes an option in a 'Column' single-select field,\n" +
			"and each card is migrated as a draft issue with the Column field set.\n\n" +
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

			// Extract owner from URL if provided.
			if projectURL, _ := parser.ParseProjectURL(args[0]); projectURL != nil {
				if srcOwnerFlag == "" {
					srcOwnerFlag = projectURL.Host + "/" + projectURL.Owner
				}
			}

			var srcOwner, srcRepoName string
			if srcRepoFlag != "" {
				srcRepo, err := parser.Repository(parser.RepositoryInput(srcRepoFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve source repository: %w", err)
				}
				srcOwner = srcRepo.Owner
				srcRepoName = srcRepo.Name
			} else {
				srcRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(srcOwnerFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve source owner: %w", err)
				}
				srcOwner = srcRepo.Owner
			}

			if dstOwnerFlag == "" {
				return fmt.Errorf("destination owner is required: use --dst")
			}
			dstRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(dstOwnerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve destination owner: %w", err)
			}

			srcClientRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(srcOwnerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve source owner for client: %w", err)
			}
			srcClient, dstClient, err := gh.NewGitHubClientWith2Repos(srcClientRepo, dstRepo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub clients: %w", err)
			}

			migrateOpts := &projects.MigrateV1Options{
				Overwrite: overwrite,
				Prune:     prune,
			}
			ctx := cmd.Context()
			p, err := projects.MigrateProjectV1ToV2(ctx, srcClient, dstClient, srcOwner, srcRepoName, dstRepo.Owner, srcNumber, migrateOpts)
			if err != nil {
				return fmt.Errorf("failed to migrate classic project #%d from '%s' to '%s': %w", srcNumber, srcOwner, dstRepo.Owner, err)
			}
			logger.Info("Migrated classic project", "srcNumber", srcNumber, "srcOwner", srcOwner, "dstNumber", p.Number, "dstOwner", dstRepo.Owner, "url", p.URL)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVarP(&srcOwnerFlag, "owner", "o", "", "Source owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVarP(&srcRepoFlag, "repo", "R", "", "Source repository in the format '[HOST/]OWNER/REPO'; for repository-scoped classic projects")
	f.StringVarP(&dstOwnerFlag, "dst", "d", "", "Destination owner in the format '[HOST/]OWNER' (required)")
	f.BoolVar(&overwrite, "overwrite", false, "Re-migrate already-migrated items instead of skipping them")
	f.BoolVar(&prune, "prune", false, "Delete previously migrated destination projects before migrating (destructive)")
	_ = f.MarkHidden("prune")
	return cmd
}
