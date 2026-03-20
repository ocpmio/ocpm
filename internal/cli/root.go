package cli

import (
	"io"

	"github.com/marian2js/ocpm/internal/auth"
	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/openclaw"
	"github.com/marian2js/ocpm/internal/publish"
	"github.com/marian2js/ocpm/internal/version"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Build    version.Info
	Install  *install.Service
	OpenClaw openclaw.Client
	Publish  *publish.Service
	Auth     *auth.Service
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ocpm",
		Short:         "Manage reusable OpenClaw agent packages",
		Long:          "ocpm installs and manages reusable OpenClaw packages for existing workspaces and newly created workspaces.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetIn(deps.Stdin)
	cmd.SetOut(deps.Stdout)
	cmd.SetErr(deps.Stderr)
	cmd.AddGroup(
		&cobra.Group{ID: "core", Title: "Core Commands:"},
		&cobra.Group{ID: "support", Title: "Support Commands:"},
	)

	cmd.AddCommand(
		newAddCommand(deps),
		newInstallCommand(deps),
		newCreateCommand(deps),
		newRemoveCommand(deps),
		newUpdateCommand(deps),
		newListCommand(deps),
		newDoctorCommand(deps),
		newInitCommand(deps),
		newPackCommand(deps),
		newPublishCommand(deps),
		newAuthCommand(deps),
		newConfigCommand(),
		newVersionCommand(deps.Build),
	)

	return cmd
}
