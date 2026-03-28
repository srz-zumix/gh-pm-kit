// Copyright (c) 2025 srz_zumix
package discussions

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

type ListOptions struct {
	Exporter cmdutil.Exporter
}

func NewListCmd() *cobra.Command {
	opts := &ListOptions{}
	var colorFlag string
	var repo string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discussions in a repository",
		Long:  "List discussions in a repository.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := parser.Repository(parser.RepositoryInput(repo))
			if err != nil {
				return fmt.Errorf("failed to resolve repository: %w", err)
			}
			client, err := gh.NewGitHubClientWithRepo(repository)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}
			ctx := context.Background()
			discussions, err := gh.ListDiscussions(ctx, client, repository, 100)
			if err != nil {
				return fmt.Errorf("failed to list discussions: %w", err)
			}
			renderer := render.NewRenderer(opts.Exporter)
			renderer.SetColor(colorFlag)
			return renderer.RenderDiscussions(discussions, nil)
		},
	}
	f := cmd.Flags()
	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", render.ColorFlagAuto, render.ColorFlags, "Use color in output")
	f.StringVarP(&repo, "repo", "R", "", "Repository in the format 'owner/repo'")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
