package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marian2js/ocpm/internal/install"
	"github.com/marian2js/ocpm/internal/registry"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/spf13/cobra"
)

const (
	installTargetCurrentPath = "current-path"
	installTargetOpenClaw    = "openclaw"
)

func newInstallCommand(deps Dependencies) *cobra.Command {
	var flags commonFlags
	var version string
	var target string
	var dir string
	var agentName string

	cmd := &cobra.Command{
		Use:     "install <package>",
		Short:   "Install a package with a guided target flow",
		GroupID: "core",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := parseOptions(flags.options)
			if err != nil {
				return err
			}

			prompter := ui.StdioPrompter{In: cmd.InOrStdin(), Out: cmd.ErrOrStderr()}
			pkg, err := deps.Install.ResolvePackage(cmd.Context(), args[0], version)
			if err != nil {
				return err
			}

			if pkg.Kind == registry.KindSkill || pkg.Kind == registry.KindOverlay {
				if target != "" || dir != "" || agentName != "" {
					return fmt.Errorf("install target prompts are only used for agent and workspace-template packages; use ocpm add for %s packages", pkg.Kind)
				}
				result, err := deps.Install.Add(cmd.Context(), install.ChangeRequest{
					WorkspacePath:  flags.workspace,
					Cwd:            mustGetwd(),
					Package:        args[0],
					Version:        version,
					PackageOptions: options,
					DryRun:         flags.dryRun,
					Prompter:       prompter,
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "package\t%s\t%s\n", result.Package, result.PackageKind)
				printOperations(cmd.OutOrStdout(), result.Operations, result.Skipped)
				return nil
			}

			selectedTarget, err := resolveInstallTarget(cmd.Context(), prompter, target, pkg, deps)
			if err != nil {
				return err
			}

			switch selectedTarget {
			case installTargetCurrentPath:
				targetDir, err := resolveCurrentPathTarget(prompter, dir, args[0], mustGetwd())
				if err != nil {
					return err
				}
				result, err := deps.Install.Create(cmd.Context(), install.CreateRequest{
					Cwd:            mustGetwd(),
					Dir:            targetDir,
					Package:        args[0],
					Version:        version,
					PackageOptions: options,
					DryRun:         flags.dryRun,
					Prompter:       prompter,
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "target\t%s\n", installTargetCurrentPath)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "package\t%s\t%s\n", result.Package, result.PackageKind)
				printOperations(cmd.OutOrStdout(), result.Operations, result.Skipped)
				return nil
			case installTargetOpenClaw:
				if deps.OpenClaw == nil || !deps.OpenClaw.IsInstalled(cmd.Context()) {
					return fmt.Errorf("openclaw is not installed; choose current path or install OpenClaw first")
				}
				resolvedAgentName, workspacePath, err := resolveOpenClawTarget(cmd.Context(), prompter, deps, agentName, dir, args[0], cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				result, err := deps.Install.InstallToOpenClaw(cmd.Context(), install.OpenClawInstallRequest{
					Cwd:            mustGetwd(),
					Package:        args[0],
					Version:        version,
					AgentName:      resolvedAgentName,
					WorkspacePath:  workspacePath,
					PackageOptions: options,
					DryRun:         flags.dryRun,
					Prompter:       prompter,
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "target\t%s\n", installTargetOpenClaw)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "agent\t%s\n", resolvedAgentName)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "workspace\t%s\n", result.WorkspacePath)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "package\t%s\t%s\n", result.Package, result.PackageKind)
				printOperations(cmd.OutOrStdout(), result.Operations, result.Skipped)
				return nil
			default:
				return fmt.Errorf("unsupported install target %q", selectedTarget)
			}
		},
	}

	cmd.Flags().StringVar(&flags.workspace, "workspace", "", "Explicit workspace path for skill and overlay installs")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview changes without writing files")
	cmd.Flags().StringArrayVar(&flags.options, "option", nil, "Package option in key=value form")
	cmd.Flags().StringVar(&version, "version", "", "Exact package version to install")
	cmd.Flags().StringVar(&target, "target", "", "Install target for agent packages: current-path or openclaw")
	cmd.Flags().StringVar(&dir, "dir", "", "Destination directory or OpenClaw workspace path")
	cmd.Flags().StringVar(&agentName, "agent-name", "", "OpenClaw agent name override")
	cmd.Aliases = []string{"i"}
	return cmd
}

func resolveInstallTarget(ctx context.Context, prompter ui.StdioPrompter, target string, pkg registry.PackageVersion, deps Dependencies) (string, error) {
	if target != "" {
		switch target {
		case installTargetCurrentPath, installTargetOpenClaw:
			return target, nil
		default:
			return "", fmt.Errorf("invalid --target value %q; use current-path or openclaw", target)
		}
	}

	if !prompter.Interactive() {
		return "", fmt.Errorf("%s is a %s package; pass --target current-path or --target openclaw", pkg.Name, pkg.Kind)
	}

	options := []ui.SelectOption{
		{
			Value: installTargetCurrentPath,
			Label: "current path",
			Hint:  fmt.Sprintf("Create ./%s in this directory", defaultInstallDirName(pkg.Name)),
		},
	}
	if deps.OpenClaw != nil && deps.OpenClaw.IsInstalled(ctx) {
		options = append(options, ui.SelectOption{
			Value: installTargetOpenClaw,
			Label: "openclaw",
			Hint:  "Register an OpenClaw agent and replace its workspace",
		})
	}
	if len(options) == 1 {
		return options[0].Value, nil
	}

	return prompter.PromptSelect("Install target", options, installTargetCurrentPath)
}

func resolveCurrentPathTarget(prompter ui.StdioPrompter, dir, packageName, cwd string) (string, error) {
	defaultDir := filepath.Join(cwd, defaultInstallDirName(packageName))
	if dir != "" {
		return filepath.Clean(dir), nil
	}
	if !prompter.Interactive() {
		return defaultDir, nil
	}
	_, _ = fmt.Fprintln(prompter.Out, "")
	return prompter.PromptText("Current path destination", defaultDir)
}

func resolveOpenClawTarget(ctx context.Context, prompter ui.StdioPrompter, deps Dependencies, agentName, dir, packageName string, out commandWriter) (string, string, error) {
	defaultAgentName := defaultInstallDirName(packageName)
	needsAgentName := agentName == ""
	needsWorkspacePath := dir == ""
	showAgentHints := needsAgentName || needsWorkspacePath
	if deps.OpenClaw != nil && showAgentHints {
		if agents, err := deps.OpenClaw.ListAgents(ctx); err == nil && len(agents) > 0 {
			_, _ = fmt.Fprintln(out, "")
			_, _ = fmt.Fprintln(out, "OpenClaw agents")
			for _, agent := range agents {
				name := agent.ID
				if agent.IsDefault {
					name += " (default)"
				}
				_, _ = fmt.Fprintf(out, "  - %s  %s\n", name, shortenHome(agent.Workspace))
			}
		}
	}

	if needsAgentName {
		agentName = defaultAgentName
	}
	if prompter.Interactive() && needsAgentName {
		_, _ = fmt.Fprintln(out, "")
		value, err := prompter.PromptText("OpenClaw agent name", agentName)
		if err != nil {
			return "", "", err
		}
		agentName = value
	}

	defaultWorkspace := defaultOpenClawWorkspace(agentName)
	if needsWorkspacePath {
		dir = defaultWorkspace
	}
	if prompter.Interactive() && needsWorkspacePath {
		value, err := prompter.PromptText("OpenClaw workspace path", dir)
		if err != nil {
			return "", "", err
		}
		dir = value
	}

	return agentName, filepath.Clean(dir), nil
}

func defaultInstallDirName(packageName string) string {
	name := strings.TrimPrefix(packageName, "@")
	return strings.ReplaceAll(name, "/", "-")
}

func defaultOpenClawWorkspace(agentName string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return filepath.Join(".openclaw", "workspace-"+agentName)
	}
	return filepath.Join(homeDir, ".openclaw", "workspace-"+agentName)
}

func shortenHome(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return path
	}
	if path == homeDir {
		return "~"
	}
	if strings.HasPrefix(path, homeDir+string(os.PathSeparator)) {
		return "~" + string(os.PathSeparator) + strings.TrimPrefix(path, homeDir+string(os.PathSeparator))
	}
	return path
}
