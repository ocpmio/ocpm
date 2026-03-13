package openclaw

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrNotInstalled                = errors.New("openclaw is not installed")
	ErrDefaultWorkspaceUnavailable = errors.New("unable to determine default OpenClaw workspace")
)

type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type Client interface {
	IsInstalled(ctx context.Context) bool
	DefaultWorkspace(ctx context.Context) (string, error)
	SetupWorkspace(ctx context.Context, path string) error
}

type Adapter struct {
	runner Runner
}

type ExecRunner struct{}

func NewAdapter(runner Runner) *Adapter {
	return &Adapter{runner: runner}
}

func NewExecAdapter() *Adapter {
	return NewAdapter(ExecRunner{})
}

func (ExecRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("%w: %s", err, message)
		}
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (a *Adapter) IsInstalled(_ context.Context) bool {
	_, err := a.runner.LookPath("openclaw")
	return err == nil
}

func (a *Adapter) DefaultWorkspace(ctx context.Context) (string, error) {
	if !a.IsInstalled(ctx) {
		return "", ErrNotInstalled
	}

	candidates := [][]string{
		{"workspace", "path"},
		{"workspace", "default", "--print"},
		{"config", "get", "default-workspace"},
	}

	for _, args := range candidates {
		output, err := a.runner.Run(ctx, "openclaw", args...)
		if err == nil && output != "" {
			return filepath.Clean(output), nil
		}
	}

	return "", ErrDefaultWorkspaceUnavailable
}

func (a *Adapter) SetupWorkspace(ctx context.Context, path string) error {
	if !a.IsInstalled(ctx) {
		return ErrNotInstalled
	}

	_, err := a.runner.Run(ctx, "openclaw", "setup", "--workspace", path)
	return err
}
