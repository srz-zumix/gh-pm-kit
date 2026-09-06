// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/cmd/projects/status"
)

// NewStatusCmd creates the projects status command and registers subcommands.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Manage status updates on a GitHub Project v2",
		Long:  `Manage status updates on a GitHub Project v2`,
	}

	cmd.AddCommand(status.NewListCmd())
	return cmd
}
