// Package projects provides CLI commands for GitHub Projects v2 management.
package projects

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/cmd/projects/field"
)

// NewFieldCmd creates the projects field command and registers subcommands.
func NewFieldCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "field",
		Short: "Manage fields in a GitHub Project v2",
		Long:  `Manage fields in a GitHub Project v2`,
	}

	cmd.AddCommand(field.NewListCmd())
	return cmd
}
