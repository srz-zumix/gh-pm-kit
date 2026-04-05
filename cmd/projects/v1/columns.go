// Package v1 provides CLI commands for GitHub Projects (classic) v1 management.
package v1

import (
	"github.com/spf13/cobra"
	v1columns "github.com/srz-zumix/gh-pm-kit/cmd/projects/v1/columns"
)

// NewColumnsCmd creates the projects v1 columns command and registers subcommands.
func NewColumnsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "columns",
		Short: "Manage columns of a GitHub Project (classic)",
		Long:  `Manage columns of a GitHub Project (classic)`,
	}

	cmd.AddCommand(v1columns.NewListCmd())
	return cmd
}
