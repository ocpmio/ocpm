package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/spf13/cobra"
)

func newRemoveCommand(deps Dependencies) *cobra.Command {
	var flags commonFlags

	cmd := &cobra.Command{
		Use:     "remove <package>",
		Aliases: []string{"rm"},
		Short:   "Remove a top-level package from a workspace",
		GroupID: "core",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Install.Remove(cmd.Context(), install.ChangeRequest{
				WorkspacePath: flags.workspace,
				Cwd:           mustGetwd(),
				Package:       args[0],
				DryRun:        flags.dryRun,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
			printOperations(cmd.OutOrStdout(), result.Operations, result.Skipped)
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.workspace, "workspace", "", "Explicit workspace path")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview changes without writing files")
	return cmd
}
