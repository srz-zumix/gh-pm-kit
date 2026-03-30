// Copyright (c) 2025 srz_zumix
package discussions

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/discussions"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

type MigrateOptions struct {
	Exporter cmdutil.Exporter
}

func NewMigrateCmd() *cobra.Command {
	opts := &MigrateOptions{}
	var colorFlag string
	var repo string
	var dstRepo string
	var number string
	var categorySlug string
	var enableDiscussions bool
	var overwrite bool
	var purge bool
	var noReactions bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate discussions to another repository",
		Long: "Migrate discussions from one repository to another (supports cross-host migration).\n" +
			"When --number is specified, only that discussion is migrated.\n" +
			"When --number is omitted, all discussions are migrated.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srcRepository, err := parser.Repository(parser.RepositoryInput(repo))
			if err != nil {
				return fmt.Errorf("failed to resolve source repository: %w", err)
			}
			dstRepository, err := parser.Repository(parser.RepositoryInput(dstRepo))
			if err != nil {
				return fmt.Errorf("failed to resolve destination repository: %w", err)
			}

			srcClient, dstClient, err := gh.NewGitHubClientWith2Repos(srcRepository, dstRepository)
			if err != nil {
				return fmt.Errorf("failed to create GitHub clients: %w", err)
			}

			migrateOpts := &discussions.MigrateOptions{
				CategorySlug:      categorySlug,
				EnableDiscussions: enableDiscussions,
				Overwrite:         overwrite,
				Purge:             purge,
				IncludeReactions:  !noReactions,
			}

			ctx := cmd.Context()
			renderer := render.NewRenderer(opts.Exporter)
			renderer.SetColor(colorFlag)

			if number != "" {
				discussion, err := discussions.MigrateDiscussion(ctx, srcClient, dstClient, srcRepository, dstRepository, number, migrateOpts)
				if err != nil {
					return fmt.Errorf("failed to migrate discussion: %w", err)
				}
				return renderer.RenderDiscussions([]gh.Discussion{*discussion}, nil)
			} else {
				result, err := discussions.MigrateDiscussions(ctx, srcClient, dstClient, srcRepository, dstRepository, migrateOpts)
				if err != nil {
					return fmt.Errorf("failed to migrate discussions: %w", err)
				}
				rendered := make([]gh.Discussion, len(result))
				for i, d := range result {
					rendered[i] = *d
				}
				return renderer.RenderDiscussions(rendered, nil)
			}
		},
	}
	f := cmd.Flags()
	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", render.ColorFlagAuto, render.ColorFlags, "Use color in output")
	f.StringVarP(&repo, "repo", "R", "", "Source repository in the format '[HOST/]OWNER/REPO' (defaults to current repository)")
	f.StringVarP(&dstRepo, "dst", "d", "", "Destination repository in the format '[HOST/]OWNER/REPO'")
	_ = cmd.MarkFlagRequired("dst")
	f.StringVarP(&number, "number", "n", "", "Discussion number or URL to migrate (migrates all if omitted)")
	f.StringVar(&categorySlug, "category", "", "Override destination category slug (uses source category slug if omitted)")
	f.BoolVar(&noReactions, "no-reactions", false, "Do not embed reaction summaries into migrated discussion and comment bodies")
	f.BoolVar(&enableDiscussions, "enable-discussions", false, "Enable Discussions on the destination repository if not already enabled")
	f.BoolVar(&overwrite, "overwrite", false, "Overwrite the contents of a previously migrated discussion when a migration marker is found (independent of title); without this flag, such marked discussions are skipped and unmarked discussions are left untouched (a new discussion is created alongside them)")
	f.BoolVar(&purge, "purge", false, "Delete ALL discussions matching the source title before migrating (destructive; overrides --overwrite)")
	_ = cmd.Flags().MarkHidden("purge")
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	return cmd
}
