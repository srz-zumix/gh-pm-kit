// Copyright (c) 2025 srz_zumix
package discussions

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/google/go-github/v90/github"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

type ReposOptions struct {
	Exporter cmdutil.Exporter
}

func NewReposCmd() *cobra.Command {
	opts := &ReposOptions{}
	var colorFlag string
	var owner string
	var all bool
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "List repositories with Discussions enabled",
		Long:  "List repositories owned by an owner that have Discussions enabled. Use --all to list every repository regardless of the Discussions setting.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := parser.Repository(parser.RepositoryOwnerWithHost(owner))
			if err != nil {
				return fmt.Errorf("failed to resolve owner: %w", err)
			}
			client, err := gh.NewGitHubClientWithRepo(repository)
			if err != nil {
				return fmt.Errorf("failed to create GitHub client: %w", err)
			}
			ctx := cmd.Context()
			repos, err := gh.ListOwnerRepositories(ctx, client, repository.Owner)
			if err != nil {
				return fmt.Errorf("failed to list repositories for owner '%s': %w", repository.Owner, err)
			}
			if !all {
				repos = filterReposWithDiscussions(repos)
			}
			renderer := render.NewRenderer(opts.Exporter)
			renderer.SetColor(colorFlag)
			return renderer.RenderRepository(repos, []string{"REPOSITORY", "VISIBILITY", "DISCUSSIONS"})
		},
	}
	f := cmd.Flags()
	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", render.ColorFlagAuto, render.ColorFlags, "Use color in output")
	f.StringVar(&owner, "owner", "", "Owner in the format '[HOST/]OWNER'. Defaults to the owner of the current repository.")
	f.BoolVar(&all, "all", false, "List all repositories, including those without Discussions enabled")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}

func filterReposWithDiscussions(repos []*github.Repository) []*github.Repository {
	filtered := make([]*github.Repository, 0, len(repos))
	for _, r := range repos {
		if r.GetHasDiscussions() {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
