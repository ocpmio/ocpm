package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/marian2js/ocpm/internal/auth"
	"github.com/marian2js/ocpm/internal/cli"
	"github.com/marian2js/ocpm/internal/config"
	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/openclaw"
	"github.com/marian2js/ocpm/internal/publish"
	"github.com/marian2js/ocpm/internal/registry"
	"github.com/marian2js/ocpm/internal/version"
	"github.com/marian2js/ocpm/internal/workspace"
)

// Run executes the CLI and converts command failures into process exit codes.
func Run(ctx context.Context, stdout io.Writer, stderr io.Writer, info version.Info, args []string) int {
	openclawClient := openclaw.NewExecAdapter()
	registryClient := registry.NewFixtureRegistry()
	service := install.NewService(
		registryClient,
		workspace.NewResolver(openclawClient),
		openclawClient,
	)
	publishService := publish.NewService(registryClient)
	paths, err := config.Resolve(config.DefaultAppName)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	authService := auth.NewService(
		auth.FileStore{Path: filepath.Join(paths.ConfigDir, "auth.json")},
		config.ResolveAuthURL(),
		nil,
	)

	cmd := cli.NewRootCommand(cli.Dependencies{
		Stdin:   os.Stdin,
		Stdout:  stdout,
		Stderr:  stderr,
		Build:   info,
		Install: service,
		Publish: publishService,
		Auth:    authService,
	})
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}
