package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

func newUpdateCommand(deps Dependencies) *cobra.Command {
	var flags commonFlags
	var version string

	cmd := &cobra.Command{
		Use:     "update [package]",
		Short:   "Update one package or all packages in a workspace",
		GroupID: "core",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := parseOptions(flags.options)
			if err != nil {
				return err
			}

			target := ""
			if len(args) == 1 {
				target = args[0]
			}

			result, err := deps.Install.Update(cmd.Context(), install.UpdateRequest{
				WorkspacePath:  flags.workspace,
				Cwd:            mustGetwd(),
				Package:        target,
				Version:        version,
				PackageOptions: options,
				DryRun:         flags.dryRun,
				Prompter:       ui.StdioPrompter{In: cmd.InOrStdin(), Out: cmd.ErrOrStderr()},
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
			if result.Package != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "package\t%s\t%s\n", result.Package, result.PackageKind)
			}
			printOperations(cmd.OutOrStdout(), result.Operations, result.Skipped)
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.workspace, "workspace", "", "Explicit workspace path")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview changes without writing files")
	cmd.Flags().StringArrayVar(&flags.options, "option", nil, "Package option in key=value form")
	cmd.Flags().StringVar(&version, "version", "", "Exact version for the package being updated")
	return cmd
}
