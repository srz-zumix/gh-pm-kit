// Package field provides CLI commands for GitHub Project v2 fields.
package field

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects field list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	var showOptions bool
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list <number|URL>",
		Short: "List field definitions in a GitHub Project v2",
		Long: "List the field definitions of a GitHub Project v2, including built-in fields.\n\n" +
			"By default each field is shown on a single row with its select options or\n" +
			"iteration count summarized. Use --show-options to expand every select option\n" +
			"and iteration (including completed ones) onto its own row.\n\n" +
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
			fields, err := gh.ListProjectV2Fields(ctx, client, repo.Owner, number)
			if err != nil {
				return fmt.Errorf("failed to list fields for project #%d of '%s': %w", number, repo.Owner, err)
			}

			renderer := render.NewRenderer(opts.Exporter)
			if showOptions {
				return renderer.RenderProjectV2FieldOptions(fields)
			}
			return renderer.RenderProjectV2Fields(fields)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	f.BoolVar(&showOptions, "show-options", false, "Expand select options and iterations onto individual rows")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
