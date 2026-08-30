/*
Copyright © 2025 srz_zumix
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/version"
	"github.com/srz-zumix/go-gh-extension/pkg/actions"
	"github.com/srz-zumix/go-gh-extension/pkg/cmdflags"
)

var rootCmd = &cobra.Command{
	Use:     "gh-pm-kit",
	Short:   "Project management extensions for GitHub CLI",
	Long:    `Project management extensions for GitHub CLI`,
	Version: version.Version,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	if actions.IsRunsOn() {
		rootCmd.SetErrPrefix(actions.GetErrorPrefix())
	}
	cmdflags.AddPersistentFlags(rootCmd)
}
