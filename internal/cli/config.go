package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Inspect runtime configuration",
		GroupID: "support",
	}

	cmd.AddCommand(newConfigPathsCommand())
	return cmd
}

func newConfigPathsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "paths",
		Short: "Print config, cache, and state directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.Resolve(config.DefaultAppName)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "config\t%s\n", paths.ConfigDir)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "cache\t%s\n", paths.CacheDir)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "state\t%s\n", paths.StateDir)
			return nil
		},
	}

	return cmd
}
