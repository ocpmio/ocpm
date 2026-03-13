package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type mockOpenClaw struct {
	installed bool
	path      string
}

func (m mockOpenClaw) IsInstalled(context.Context) bool { return m.installed }
func (m mockOpenClaw) DefaultWorkspace(context.Context) (string, error) {
	return m.path, nil
}
func (m mockOpenClaw) SetupWorkspace(context.Context, string) error { return nil }

func TestDetectRecognizesStrongIndicators(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TOOLS.md"), []byte("# Tools\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	detection, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if !detection.LooksLikeWorkspace {
		t.Fatalf("expected workspace detection")
	}
}

func TestResolverFallsBackToOpenClawDefaultWorkspace(t *testing.T) {
	cwd := t.TempDir()
	defaultWorkspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(defaultWorkspace, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolver := NewResolver(mockOpenClaw{installed: true, path: defaultWorkspace})
	resolution, err := resolver.Resolve(context.Background(), cwd, "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Source != "openclaw-default" {
		t.Fatalf("expected openclaw-default source, got %q", resolution.Source)
	}
	if resolution.Path != defaultWorkspace {
		t.Fatalf("expected %q, got %q", defaultWorkspace, resolution.Path)
	}
}
