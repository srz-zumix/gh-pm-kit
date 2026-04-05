// Package projects provides CLI commands for GitHub Projects v1 (classic) management.
package projects

import (
	"github.com/spf13/cobra"
	projectsv1 "github.com/srz-zumix/gh-pm-kit/cmd/projects/v1"
)

// NewV1Cmd creates the projects v1 command and registers subcommands.
func NewV1Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "v1",
		Short: "Manage GitHub Projects (classic)",
		Long:  `Manage GitHub Projects (classic)`,
	}

	cmd.AddCommand(projectsv1.NewListCmd())
	cmd.AddCommand(projectsv1.NewColumnsCmd())
	cmd.AddCommand(projectsv1.NewCardsCmd())
	cmd.AddCommand(projectsv1.NewMigrateCmd())
	return cmd
}
