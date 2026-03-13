package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marian2js/ocpm/internal/manifest"
	"github.com/marian2js/ocpm/internal/version"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), &stdout, &stderr, version.New("1.2.3", "abcdef", "2026-03-13T10:00:00Z"), []string{"version"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	if !strings.Contains(stdout.String(), "1.2.3") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestRunAddInstallsFixturePackage(t *testing.T) {
	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte("# Workspace\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(currentDir)
	})
	if err := os.Chdir(workspaceDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), &stdout, &stderr, version.New("dev", "none", "unknown"), []string{"add", "@acme/browser-skill"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "@acme/browser-skill") {
		t.Fatalf("expected installed package output, got %q", stdout.String())
	}

	manifestFile, err := manifest.Read(filepath.Join(workspaceDir, manifest.FileName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if _, ok := manifestFile.Dependencies["@acme/browser-skill"]; !ok {
		t.Fatalf("expected dependency to be recorded in manifest")
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestRunPublishDryRunJSON(t *testing.T) {
	packageDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(packageDir, "ocpm.json"), []byte("{\n  \"name\": \"@acme/browser-skill\",\n  \"version\": \"0.1.0\",\n  \"kind\": \"skill\"\n}\n"), 0o644); err != nil {
		t.Fatalf("write ocpm.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(packageDir, "payload"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "payload", "data.txt"), []byte("payload\n"), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(currentDir)
	})
	if err := os.Chdir(packageDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), &stdout, &stderr, version.New("dev", "none", "unknown"), []string{"publish", "--dry-run", "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"name\": \"@acme/browser-skill\"") {
		t.Fatalf("expected publish json output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\"dryRun\": true") {
		t.Fatalf("expected dryRun json field, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}
