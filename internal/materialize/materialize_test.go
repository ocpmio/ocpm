package materialize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marian2js/ocpm/internal/lockfile"
	"github.com/marian2js/ocpm/internal/registry"
)

func TestSyncWritesSkillsAndManagedSections(t *testing.T) {
	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := Sync(SyncRequest{
		WorkspacePath: workspaceDir,
		Packages: []DesiredPackage{
			{
				Package: registry.PackageVersion{
					Name:    "@acme/browser-skill",
					Version: "1.0.0",
					Kind:    registry.KindSkill,
					Files: map[string]string{
						"metadata/package.txt": "payload\n",
					},
					Skills: []registry.Skill{
						{Name: "browser", Files: map[string]string{"SKILL.md": "skill\n"}},
					},
					ManagedFiles: []registry.ManagedFile{
						{Path: "AGENTS.md", Content: "- browser\n"},
					},
				},
			},
		},
		Now: time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}

	if len(result.Lock.Packages) != 1 {
		t.Fatalf("expected one package in lock, got %d", len(result.Lock.Packages))
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "skills", "browser", "SKILL.md")); err != nil {
		t.Fatalf("expected skill file: %v", err)
	}
	agents, err := os.ReadFile(filepath.Join(workspaceDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(agents), "ocpm:begin @acme/browser-skill") {
		t.Fatalf("expected managed markers in AGENTS.md, got %q", string(agents))
	}
}

func TestSyncRefusesToOverwriteUserOwnedFiles(t *testing.T) {
	workspaceDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaceDir, "skills", "browser"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "skills", "browser", "SKILL.md"), []byte("user\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Sync(SyncRequest{
		WorkspacePath: workspaceDir,
		Packages: []DesiredPackage{
			{
				Package: registry.PackageVersion{
					Name:    "@acme/browser-skill",
					Version: "1.0.0",
					Kind:    registry.KindSkill,
					Skills: []registry.Skill{
						{Name: "browser", Files: map[string]string{"SKILL.md": "managed\n"}},
					},
					ManagedFiles: []registry.ManagedFile{
						{Path: "AGENTS.md", Content: "- browser\n"},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "user-owned file") {
		t.Fatalf("expected user-owned overwrite error, got %v", err)
	}
}

func TestSyncSkipsRemovingModifiedFiles(t *testing.T) {
	workspaceDir := t.TempDir()
	skillPath := filepath.Join(workspaceDir, "skills", "browser", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	current := lockfile.New(workspaceDir, time.Now())
	current.Packages = []lockfile.PackageLock{
		{
			Name:    "@acme/browser-skill",
			Version: "1.0.0",
			Kind:    registry.KindSkill,
			InstalledFiles: []lockfile.InstalledFile{
				{Path: "skills/browser/SKILL.md", Integrity: "sha256:original"},
			},
		},
	}

	result, err := Sync(SyncRequest{
		WorkspacePath: workspaceDir,
		Current:       current,
	})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Action != "skip-remove" {
		t.Fatalf("expected modified file removal to be skipped, got %+v", result.Skipped)
	}
}
