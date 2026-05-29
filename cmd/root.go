// Package cmd defines the dbviz CLI built on cobra.
package cmd

import (
	"github.com/spf13/cobra"
)

// buildVersion is the version string injected from main.
var buildVersion = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "dbviz",
		Short:         "Universal database visualizer",
		Long:          "dbviz connects to any database read-only, introspects its schema, and renders it as an interactive force-directed graph.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newVersionCmd())
	return root
}

// Execute runs the root command. version is the build version string.
func Execute(version string) error {
	buildVersion = version
	return newRootCmd().Execute()
}
