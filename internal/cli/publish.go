package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/publish"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

func newPackCommand(deps Dependencies) *cobra.Command {
	var request publish.Request
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "pack",
		Short:   "Create a deterministic local package archive",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			request.Cwd = mustGetwd()
			result, err := deps.Publish.Pack(cmd.Context(), request)
			if err != nil {
				return err
			}

			if jsonOutput {
				return ui.WriteJSON(cmd.OutOrStdout(), result)
			}
			return printPublishResult(cmd.OutOrStdout(), result, request.ListFiles)
		},
	}

	bindPublishFlags(cmd, &request, &jsonOutput)
	return cmd
}

func newPublishCommand(deps Dependencies) *cobra.Command {
	var request publish.Request
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "publish",
		Short:   "Validate, package, and publish the current project",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			request.Cwd = mustGetwd()
			result, err := deps.Publish.Publish(cmd.Context(), request)
			if err != nil {
				return err
			}

			if jsonOutput {
				return ui.WriteJSON(cmd.OutOrStdout(), result)
			}
			return printPublishResult(cmd.OutOrStdout(), result, request.ListFiles)
		},
	}

	bindPublishFlags(cmd, &request, &jsonOutput)
	return cmd
}

func bindPublishFlags(cmd *cobra.Command, request *publish.Request, jsonOutput *bool) {
	cmd.Flags().StringVar(&request.Path, "path", "", "Package directory to pack or publish")
	cmd.Flags().BoolVar(&request.DryRun, "dry-run", false, "Validate and assemble without writing or uploading")
	cmd.Flags().BoolVar(jsonOutput, "json", false, "Emit machine-readable JSON")
	cmd.Flags().BoolVar(&request.Private, "private", false, "Shorthand for --access private")
	cmd.Flags().StringVar(&request.Tag, "tag", "latest", "Publish tag")
	cmd.Flags().StringVar(&request.RegistryURL, "registry", "", "Registry URL override")
	cmd.Flags().StringVar(&request.Out, "out", "", "Write the tarball to a local file")
	cmd.Flags().StringVar(&request.Access, "access", "", "Package access level: public or private")
	cmd.Flags().BoolVar(&request.ListFiles, "list", false, "Print packaged file contents")
}

func printPublishResult(out commandWriter, result publish.Result, listFiles bool) error {
	_, _ = fmt.Fprintf(out, "name\t%s\n", result.Name)
	_, _ = fmt.Fprintf(out, "version\t%s\n", result.Version)
	_, _ = fmt.Fprintf(out, "kind\t%s\n", result.Kind)
	_, _ = fmt.Fprintf(out, "tag\t%s\n", result.Tag)
	_, _ = fmt.Fprintf(out, "access\t%s\n", result.Access)
	_, _ = fmt.Fprintf(out, "registry\t%s\n", result.RegistryURL)
	if result.PackageURL != "" {
		_, _ = fmt.Fprintf(out, "package-url\t%s\n", result.PackageURL)
	}
	if result.ArchivePath != "" {
		_, _ = fmt.Fprintf(out, "archive\t%s\n", result.ArchivePath)
	}
	_, _ = fmt.Fprintf(out, "files\t%d\n", result.FileCount)
	_, _ = fmt.Fprintf(out, "archive-bytes\t%d\n", result.ArchiveBytes)
	_, _ = fmt.Fprintf(out, "uncompressed-bytes\t%d\n", result.UncompressedBytes)
	_, _ = fmt.Fprintf(out, "sha256\t%s\n", result.SHA256)
	_, _ = fmt.Fprintf(out, "integrity\t%s\n", result.Integrity)
	_, _ = fmt.Fprintf(out, "dry-run\t%t\n", result.DryRun)
	_, _ = fmt.Fprintf(out, "uploaded\t%t\n", result.Uploaded)
	_, _ = fmt.Fprintf(out, "packed\t%t\n", result.Packed)
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintf(out, "warning\t%s\n", warning)
	}
	if listFiles {
		for _, file := range result.Files {
			_, _ = fmt.Fprintf(out, "file\t%s\t%d\t%s\n", file.Path, file.Bytes, file.SHA256)
		}
	}
	return nil
}
