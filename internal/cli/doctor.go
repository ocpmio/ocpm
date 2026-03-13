package cli

import (
	"fmt"
	"strings"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

func newDoctorCommand(deps Dependencies) *cobra.Command {
	var workspacePath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Diagnose workspace targeting, OpenClaw integration, and ocpm state",
		GroupID: "support",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Install.Doctor(cmd.Context(), install.DoctorRequest{
				WorkspacePath: workspacePath,
				Cwd:           mustGetwd(),
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return ui.WriteJSON(cmd.OutOrStdout(), result)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "path\t%s\n", result.CurrentPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%t\n", result.Workspace.LooksLikeWorkspace)
			if len(result.Workspace.Indicators) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "indicators\t%s\n", strings.Join(result.Workspace.Indicators, ","))
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "openclaw\t%t\n", result.OpenClawInstalled)
			if result.DefaultWorkspace != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "default-workspace\t%s\n", result.DefaultWorkspace)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "manifest\t%t\n", result.ManifestExists)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "lockfile\t%t\n", result.LockfileExists)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "consistent\t%t\n", result.ManifestLockConsistent)
			for _, issue := range result.Issues {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "issue\t%s\n", issue)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workspacePath, "workspace", "", "Inspect a specific workspace path")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON")
	return cmd
}
