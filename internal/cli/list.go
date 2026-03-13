package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

func newListCommand(deps Dependencies) *cobra.Command {
	var workspacePath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed packages in a workspace",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Install.List(cmd.Context(), install.ListRequest{
				WorkspacePath: workspacePath,
				Cwd:           mustGetwd(),
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return ui.WriteJSON(cmd.OutOrStdout(), result)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
			if len(result.Packages) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no packages")
				return nil
			}
			for _, pkg := range result.Packages {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", pkg.Name, pkg.Version, pkg.Kind, pkg.Status)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workspacePath, "workspace", "", "Explicit workspace path")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON")
	return cmd
}
