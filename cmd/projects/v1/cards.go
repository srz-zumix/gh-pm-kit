// Package v1 provides CLI commands for GitHub Projects (classic) v1 management.
package v1

import (
	"github.com/spf13/cobra"
	v1cards "github.com/srz-zumix/gh-pm-kit/cmd/projects/v1/cards"
)

// NewCardsCmd creates the projects v1 cards command and registers subcommands.
func NewCardsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cards",
		Short: "Manage cards in a GitHub Project (classic) column",
		Long:  `Manage cards in a GitHub Project (classic) column`,
	}

	cmd.AddCommand(v1cards.NewListCmd())
	return cmd
}
