// Package item provides the CLI command for listing GitHub Project v2 items.
package item

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewListCmd creates the projects item list command.
func NewListCmd() *cobra.Command {
	var ownerFlag string
	var fields []string
	var customFields []string
	opts := struct {
		Exporter cmdutil.Exporter
	}{}

	cmd := &cobra.Command{
		Use:   "list <number|URL>",
		Short: "List item in a GitHub Project v2",
		Long: "List item in a GitHub Project v2.\n\n" +
			"The project can be specified by its number or by its URL\n" +
			"(e.g. https://github.com/orgs/my-org/projects/1).\n\n" +
			"Owner format: '[HOST/]OWNER' (e.g. 'my-org' or 'github.com/my-org').",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := parser.GetProjectNumberFromString(args[0])
			if err != nil {
				return fmt.Errorf("invalid project number or URL %q: %w", args[0], err)
			}
			owner := ownerFlag
			if projectURL, _ := parser.ParseProjectURL(args[0]); projectURL != nil {
				owner = projectURL.Host + "/" + projectURL.Owner
			}
			repo, err := parser.Repository(parser.RepositoryOwnerWithHost(owner))
			if err != nil {
				return fmt.Errorf("failed to resolve owner: %w", err)
			}

			client, err := gh.NewGitHubClientWithRepo(repo)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}

			ctx := cmd.Context()
			items, err := gh.ListProjectV2Items(ctx, client, repo.Owner, number)
			if err != nil {
				return fmt.Errorf("failed to list items for project #%d of '%s': %w", number, repo.Owner, err)
			}

			allFields := append(fields, customFields...)
			renderer := render.NewRenderer(opts.Exporter)
			return renderer.RenderProjectV2Items(items, allFields)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&ownerFlag, "owner", "o", "", "Owner in the format '[HOST/]OWNER' (defaults to current repository owner)")
	cmdutil.StringSliceEnumFlag(cmd, &fields, "field", "", nil, render.ProjectV2ItemFields, "Fields to display (default: TYPE,NUMBER,TITLE,URL)")
	f.StringSliceVar(&customFields, "custom-field", nil, "Custom field names to display (any ProjectV2 custom field name)")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
