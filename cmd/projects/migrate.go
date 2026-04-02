// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"fmt"

	"github.com/spf13/cobra"
	pkgprojects "github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
)

// NewMigrateCmd creates the projects migrate command.
func NewMigrateCmd() *cobra.Command {
	var srcOwnerFlag string
	var dstOwnerFlag string
	var issueRepoFlag string
	var createIssue bool
	var overwrite bool
	var prune bool

	cmd := &cobra.Command{
		Use:   "migrate <number|URL> [dst-number|dst-URL]",
		Short: "Migrate a GitHub Project v2 to another owner",
		Long: "Migrate a GitHub Project v2 (New Projects) from one owner to another.\n" +
			"The source project metadata, custom fields (TEXT, NUMBER, DATE, SINGLE_SELECT),\n" +
			"and items (migrated as draft issues) are copied to the destination owner.\n\n" +
			"The source project can be specified by its number or by its URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"If a destination project number or URL is given as the second argument,\n" +
			"that project is always overwritten. Without a destination project,\n" +
			"a new project is created (unless --overwrite is given).\n\n" +
			"Items already migrated are identified by a hidden marker and skipped\n" +
			"unless --overwrite is specified.\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcNumber, err := parser.GetProjectNumberFromString(args[0])
			if err != nil {
				return fmt.Errorf("invalid source project number or URL %q: %w", args[0], err)
			}
			srcOwner := srcOwnerFlag
			if projectURL, _ := parser.ParseProjectURL(args[0]); projectURL != nil {
				srcOwner = projectURL.Host + "/" + projectURL.Owner
			}
			srcRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(srcOwner))
			if err != nil {
				return fmt.Errorf("failed to resolve source owner: %w", err)
			}

			// Parse optional destination project argument.
			var dstNumber int
			hasDstProject := len(args) == 2
			if hasDstProject {
				dstNumber, err = parser.GetProjectNumberFromString(args[1])
				if err != nil {
					return fmt.Errorf("invalid destination project number or URL %q: %w", args[1], err)
				}
				if projectURL, _ := parser.ParseProjectURL(args[1]); projectURL != nil {
					if dstOwnerFlag == "" {
						dstOwnerFlag = projectURL.Host + "/" + projectURL.Owner
					}
				}
			}

			if dstOwnerFlag == "" {
				return fmt.Errorf("destination owner is required: use --dst or provide a destination project URL")
			}
			dstRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(dstOwnerFlag))
			if err != nil {
				return fmt.Errorf("failed to resolve destination owner: %w", err)
			}

			srcClient, dstClient, err := gh.NewGitHubClientWith2Repos(srcRepo, dstRepo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub clients: %w", err)
			}

			migrateOpts := &pkgprojects.MigrateOptions{
				Overwrite: overwrite,
				Prune:     prune,
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

			if hasDstProject {
				p, migrateErr := pkgprojects.MigrateProjectTo(
					ctx, srcClient, dstClient, srcRepo.Owner, dstRepo.Owner, srcNumber, dstNumber, migrateOpts,
				)
				if migrateErr != nil {
					return fmt.Errorf("failed to migrate project #%d from '%s' to project #%d of '%s': %w",
						srcNumber, srcRepo.Owner, dstNumber, dstRepo.Owner, migrateErr)
				}
				logger.Info("Migrated project", "url", p.URL)
			} else {
				p, migrateErr := pkgprojects.MigrateProject(
					ctx, srcClient, dstClient, srcRepo.Owner, dstRepo.Owner, srcNumber, migrateOpts,
				)
				if migrateErr != nil {
					return fmt.Errorf("failed to migrate project #%d from '%s' to '%s': %w",
						srcNumber, srcRepo.Owner, dstRepo.Owner, migrateErr)
				}
				logger.Info("Migrated project", "url", p.URL)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVarP(&srcOwnerFlag, "src", "s", "", "Source owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVarP(&dstOwnerFlag, "dst", "d", "", "Destination owner in the format '[HOST/]OWNER' (required unless a destination URL is given as the second argument)")
	f.StringVarP(&issueRepoFlag, "repo", "r", "", "Repository in '[HOST/]OWNER/REPO' format; items are linked to matching issues (by migration marker) in this repository")
	f.BoolVar(&createIssue, "create-issue", false, "When --repo is set and no matching issue is found, create a new issue instead of a draft issue")
	f.BoolVar(&overwrite, "overwrite", false, "Delete and re-create a previously migrated item when a migration marker is found; without this option, such items are skipped")
	f.BoolVar(&prune, "prune", false, "Delete ALL previously migrated projects and items from the destination before migrating (destructive; overrides --overwrite)")
	_ = f.MarkHidden("prune")
	return cmd
}
