package cli

import (
	"fmt"
	"os"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

func newAddCommand(deps Dependencies) *cobra.Command {
	return newAddLikeCommand(deps, "add", "Add a package into an existing workspace")
}

func newAddLikeCommand(deps Dependencies, use, short string) *cobra.Command {
	var flags commonFlags
	var version string
	var allowUnsafe bool

	cmd := &cobra.Command{
		Use:     use + " <package>",
		Short:   short,
		GroupID: "core",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := parseOptions(flags.options)
			if err != nil {
				return err
			}

			result, err := deps.Install.Add(cmd.Context(), install.ChangeRequest{
				WorkspacePath:    flags.workspace,
				Cwd:              mustGetwd(),
				Package:          args[0],
				Version:          version,
				PackageOptions:   options,
				DryRun:           flags.dryRun,
				AllowUnsafeKinds: allowUnsafe,
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

	cmd.Flags().StringVar(&flags.workspace, "workspace", "", "Explicit workspace path")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview changes without writing files")
	cmd.Flags().StringArrayVar(&flags.options, "option", nil, "Package option in key=value form")
	cmd.Flags().StringVar(&version, "version", "", "Exact package version to install")
	cmd.Flags().BoolVar(&allowUnsafe, "allow-unsafe-kind", false, "Allow package kinds that normally require ocpm create")
	return cmd
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
