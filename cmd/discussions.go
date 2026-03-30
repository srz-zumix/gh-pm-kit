/*
Copyright © 2025 srz_zumix
*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/cmd/discussions"
)

// NewDiscussionsCmd creates the discussions command and registers subcommands
func NewDiscussionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discussions",
		Short: "Manage GitHub Discussions",
		Long:  `Manage GitHub Discussions`,
	}

	cmd.AddCommand(discussions.NewListCmd())
	cmd.AddCommand(discussions.NewSearchCmd())
	cmd.AddCommand(discussions.NewMigrateCmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewDiscussionsCmd())
}
