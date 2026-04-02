// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/cmd/projects/item"
)

// NewItemCmd creates the projects item command and registers subcommands.
func NewItemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "item",
		Short: "Manage item in a GitHub Project v2",
		Long:  `Manage item in a GitHub Project v2`,
	}

	cmd.AddCommand(item.NewListCmd())
	return cmd
}
