/*
Copyright © 2025 srz_zumix
*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/cmd/projects"
)

// NewProjectsCmd creates the projects command and registers subcommands.
func NewProjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Manage GitHub Projects (v2)",
		Long:  `Manage GitHub Projects (v2)`,
	}

	cmd.AddCommand(projects.NewListCmd())
	cmd.AddCommand(projects.NewItemCmd())
	cmd.AddCommand(projects.NewMigrateCmd())
	cmd.AddCommand(projects.NewV1Cmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewProjectsCmd())
}
