package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/registry"
	"github.com/spf13/cobra"
)

func newInitCommand(deps Dependencies) *cobra.Command {
	var workspacePath string
	var workspaceManifest bool
	var name string
	var version string
	var kind string
	var private bool
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize ocpm.json in the current directory or a target path",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Install.Init(cmd.Context(), install.InitRequest{
				Path:              workspacePath,
				Cwd:               mustGetwd(),
				WorkspaceManifest: workspaceManifest,
				Name:              name,
				Version:           version,
				Kind:              registry.PackageKind(kind),
				Private:           private,
				Force:             force,
				DryRun:            dryRun,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
			printOperations(cmd.OutOrStdout(), result.Operations, nil)
			return nil
		},
	}

	cmd.Flags().StringVar(&workspacePath, "workspace", "", "Path to initialize")
	cmd.Flags().BoolVar(&workspaceManifest, "workspace-manifest", false, "Mark the manifest as describing a workspace")
	cmd.Flags().StringVar(&name, "name", "", "Package name override")
	cmd.Flags().StringVar(&version, "version", "", "Package version override")
	cmd.Flags().StringVar(&kind, "kind", "", "Package kind override: skill, overlay, workspace-template, or agent")
	cmd.Flags().BoolVar(&private, "private", false, "Mark the initialized package as private")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite ocpm.json if it already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing files")
	return cmd
}
