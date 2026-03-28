// Copyright (c) 2025 srz_zumix
package discussions

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

type SearchOptions struct {
	Exporter cmdutil.Exporter
}

func NewSearchCmd() *cobra.Command {
	opts := &SearchOptions{}
	var colorFlag string
	var repo string
	var owner string
	var labels []string
	cmd := &cobra.Command{
		Use:   "search [query...]",
		Short: "Search discussions by query",
		Long:  "Search discussions in the repository using a search query. The query can include label filters and other search criteria.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			for _, label := range labels {
				query += fmt.Sprintf(" label:%q", label)
			}
			repository, err := parser.Repository(parser.RepositoryOwner(owner), parser.RepositoryInput(repo))
			if err != nil {
				return fmt.Errorf("failed to resolve repository: %w", err)
			}
			client, err := gh.NewGitHubClientWithRepo(repository)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}
			ctx := cmd.Context()
			discussions, err := gh.SearchDiscussions(ctx, client, repository, query)
			if err != nil {
				return fmt.Errorf("failed to search discussions: %w", err)
			}
			renderer := render.NewRenderer(opts.Exporter)
			renderer.SetColor(colorFlag)
			return renderer.RenderDiscussions(discussions, nil)
		},
	}
	f := cmd.Flags()
	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", render.ColorFlagAuto, render.ColorFlags, "Use color in output")
	f.StringVarP(&repo, "repo", "R", "", "Repository in the format '[HOST/]OWNER/REPO'. Defaults to the current repository.")
	f.StringVar(&owner, "owner", "", "Owner in the format '[HOST/]OWNER'. Defaults to the owner of the current repository.")
	f.StringSliceVarP(&labels, "label", "l", []string{}, "Filter discussions by labels")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
