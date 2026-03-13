package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marian2js/ocpm/internal/lockfile"
	"github.com/marian2js/ocpm/internal/registry"
	"github.com/marian2js/ocpm/internal/workspace"
)

type stubOpenClaw struct {
	installed bool
	path      string
	setupPath string
}

func (s *stubOpenClaw) IsInstalled(_ context.Context) bool { return s.installed }
func (s *stubOpenClaw) DefaultWorkspace(_ context.Context) (string, error) {
	return s.path, nil
}
func (s *stubOpenClaw) SetupWorkspace(_ context.Context, path string) error {
	s.setupPath = path
	return nil
}

func newTestService(client *stubOpenClaw) *Service {
	service := NewService(registry.NewFixtureRegistry(), workspace.NewResolver(client), client)
	service.Now = func() time.Time {
		return time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)
	}
	return service
}

func TestAddAndRemovePackagePreservesUserContent(t *testing.T) {
	openclawClient := &stubOpenClaw{}
	service := newTestService(openclawClient)
	workspaceDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte("# Agents\n\nUser notes.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := service.Add(context.Background(), ChangeRequest{
		Cwd:     workspaceDir,
		Package: "@acme/browser-skill",
	}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workspaceDir, "skills", "browser", "SKILL.md")); err != nil {
		t.Fatalf("expected skill installation: %v", err)
	}

	removeResult, err := service.Remove(context.Background(), ChangeRequest{
		Cwd:     workspaceDir,
		Package: "@acme/browser-skill",
	})
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if len(removeResult.Operations) == 0 {
		t.Fatalf("expected remove operations")
	}

	agents, err := os.ReadFile(filepath.Join(workspaceDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(agents), "browser-skill") {
		t.Fatalf("expected managed browser content to be removed, got %q", string(agents))
	}
	if !strings.Contains(string(agents), "User notes.") {
		t.Fatalf("expected user notes to remain, got %q", string(agents))
	}
}

func TestCreateBuildsFreshWorkspace(t *testing.T) {
	openclawClient := &stubOpenClaw{installed: true}
	service := newTestService(openclawClient)
	parentDir := t.TempDir()
	targetDir := filepath.Join(parentDir, "founder")

	result, err := service.Create(context.Background(), CreateRequest{
		Cwd:              parentDir,
		Dir:              targetDir,
		Package:          "@acme/founder-agent",
		RunOpenClawSetup: true,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.PackageKind != registry.KindAgent {
		t.Fatalf("expected agent kind, got %q", result.PackageKind)
	}
	if openclawClient.setupPath != targetDir {
		t.Fatalf("expected OpenClaw setup to target %q, got %q", targetDir, openclawClient.setupPath)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skills", "founder", "SKILL.md")); err != nil {
		t.Fatalf("expected founder skill: %v", err)
	}
}

func TestUpdatePreservesChosenOptions(t *testing.T) {
	service := newTestService(&stubOpenClaw{})
	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := service.Add(context.Background(), ChangeRequest{
		Cwd:            workspaceDir,
		Package:        "@acme/browser-skill",
		PackageOptions: map[string]string{"homepage": "https://custom.example"},
	}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if _, err := service.Update(context.Background(), UpdateRequest{
		Cwd: workspaceDir,
	}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	lock, err := lockfile.ReadFromDir(workspaceDir)
	if err != nil {
		t.Fatalf("ReadFromDir returned error: %v", err)
	}
	pkg, ok := lock.FindPackage("@acme/browser-skill")
	if !ok {
		t.Fatalf("expected browser package in lock")
	}
	if pkg.Options["homepage"] != "https://custom.example" {
		t.Fatalf("expected custom homepage to persist, got %+v", pkg.Options)
	}
}

func TestDoctorReportsCorruptedManagedSections(t *testing.T) {
	service := newTestService(&stubOpenClaw{})
	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte("<!-- ocpm:begin broken -->\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := service.Doctor(context.Background(), DoctorRequest{WorkspacePath: workspaceDir, Cwd: workspaceDir})
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if result.ManifestLockConsistent {
		t.Fatalf("expected doctor to flag inconsistency")
	}
	if len(result.CorruptedManagedFiles) == 0 || result.CorruptedManagedFiles[0] != "AGENTS.md" {
		t.Fatalf("expected AGENTS.md corruption to be reported, got %+v", result.CorruptedManagedFiles)
	}
}
