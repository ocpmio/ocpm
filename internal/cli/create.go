package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

func newCreateCommand(deps Dependencies) *cobra.Command {
	var dir string
	var dryRun bool
	var version string
	var options []string
	var openclawSetup bool

	cmd := &cobra.Command{
		Use:     "create <package>",
		Short:   "Create a fresh workspace from an agent or workspace template package",
		GroupID: "core",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedOptions, err := parseOptions(options)
			if err != nil {
				return err
			}

			result, err := deps.Install.Create(cmd.Context(), install.CreateRequest{
				Cwd:              mustGetwd(),
				Dir:              dir,
				Package:          args[0],
				Version:          version,
				PackageOptions:   parsedOptions,
				DryRun:           dryRun,
				RunOpenClawSetup: openclawSetup,
				Prompter:         ui.StdioPrompter{In: cmd.InOrStdin(), Out: cmd.ErrOrStderr()},
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "package\t%s\t%s\n", result.Package, result.PackageKind)
			printOperations(cmd.OutOrStdout(), result.Operations, result.Skipped)
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Target directory for the new workspace")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing files")
	cmd.Flags().StringVar(&version, "version", "", "Exact package version to create from")
	cmd.Flags().StringArrayVar(&options, "option", nil, "Package option in key=value form")
	cmd.Flags().BoolVar(&openclawSetup, "openclaw-setup", false, "Run openclaw setup --workspace <dir> after creation")
	return cmd
}
