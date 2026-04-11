// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	pkgrender "github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewDiffCmd creates the projects diff command.
func NewDiffCmd() *cobra.Command {
	var srcOwnerFlag string
	var dstOwnerFlag string
	var colorFlag string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "diff <src-number|src-URL> <dst-number|dst-URL>",
		Short: "Show the diff between a source and destination GitHub Project v2",
		Long: "Show the differences between a source and destination GitHub Project v2.\n\n" +
			"Items are matched using the migration markers embedded during migration, so\n" +
			"this command is most useful after running 'projects migrate'.\n\n" +
			"Custom fields are compared by name and type. Items are shown as:\n" +
			"  -  present only in the source (not yet migrated)\n" +
			"  +  present only in the destination (added after migration)\n" +
			"  ~  present in both but with differences (title or field values)\n\n" +
			"The source and destination projects can each be specified by their number\n" +
			"or by URL (e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcNumber, err := parser.GetProjectNumberFromString(args[0])
			if err != nil {
				return fmt.Errorf("invalid source project number or URL %q: %w", args[0], err)
			}
			srcOwner := srcOwnerFlag
			if projectURL, _ := parser.ParseProjectURL(args[0]); projectURL != nil {
				srcOwner = projectURL.Repo.Host + "/" + projectURL.Repo.Owner
			}
			srcRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(srcOwner))
			if err != nil {
				return fmt.Errorf("failed to resolve source owner: %w", err)
			}

			dstNumber, err := parser.GetProjectNumberFromString(args[1])
			if err != nil {
				return fmt.Errorf("invalid destination project number or URL %q: %w", args[1], err)
			}
			dstOwner := dstOwnerFlag
			if projectURL, _ := parser.ParseProjectURL(args[1]); projectURL != nil {
				urlDstOwner := projectURL.Repo.Host + "/" + projectURL.Repo.Owner
				if dstOwner == "" {
					dstOwner = urlDstOwner
				} else if dstOwner != urlDstOwner {
					return fmt.Errorf("destination owner mismatch: --dst %q does not match destination project URL owner %q", dstOwner, urlDstOwner)
				}
			}

			if dstOwner == "" {
				return fmt.Errorf("destination owner is required: use --dst or provide a destination project URL")
			}
			dstRepo, err := parser.Repository(parser.RepositoryOwnerWithHost(dstOwner))
			if err != nil {
				return fmt.Errorf("failed to resolve destination owner: %w", err)
			}

			srcClient, dstClient, err := gh.NewGitHubClientWith2Repos(srcRepo, dstRepo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub clients: %w", err)
			}

			ctx := cmd.Context()
			report, err := projects.DiffProjects(
				ctx, srcClient, dstClient, srcRepo.Host, srcRepo.Owner, dstRepo.Owner, srcNumber, dstNumber,
			)
			if err != nil {
				return fmt.Errorf("failed to diff projects: %w", err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			renderer.SetColor(colorFlag)
			return pkgrender.RenderProjectDiff(renderer, report)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&srcOwnerFlag, "src", "s", "", "Source owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.StringVarP(&dstOwnerFlag, "dst", "d", "", "Destination owner in the format '[HOST/]OWNER' (required unless a destination URL is given)")
	f.StringVar(&colorFlag, "color", render.ColorFlagAuto, "Colorize output ('always', 'never', 'auto')")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
