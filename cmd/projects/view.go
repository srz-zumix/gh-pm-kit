// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/cmd/projects/view"
)

// NewViewCmd creates the projects view command and registers subcommands.
func NewViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Manage views in a GitHub Project v2",
		Long:  `Manage views in a GitHub Project v2`,
	}

	cmd.AddCommand(view.NewListCmd())
	return cmd
}
