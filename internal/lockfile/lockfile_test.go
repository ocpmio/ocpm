package lockfile

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/marian2js/ocpm/internal/registry"
)

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	file := New(dir, time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC))
	file.Packages = []PackageLock{
		{
			Name:        "@acme/browser-skill",
			Version:     "1.0.0",
			Kind:        registry.KindSkill,
			Integrity:   "sha256:test",
			ResolvedURL: "mock://registry/browser",
			Options:     map[string]string{"homepage": "https://example.com"},
			InstalledFiles: []InstalledFile{
				{Path: "skills/browser/SKILL.md", Integrity: "sha256:file"},
			},
			ManagedSections: []ManagedSection{
				{File: "AGENTS.md", Owner: "@acme/browser-skill"},
			},
		},
	}

	if err := Write(path, file); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	if got.Version != SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", SchemaVersion, got.Version)
	}
	pkg, ok := got.FindPackage("@acme/browser-skill")
	if !ok {
		t.Fatalf("expected package in lockfile")
	}
	if pkg.Kind != registry.KindSkill {
		t.Fatalf("expected kind %q, got %q", registry.KindSkill, pkg.Kind)
	}
	if len(pkg.InstalledFiles) != 1 || pkg.InstalledFiles[0].Path != "skills/browser/SKILL.md" {
		t.Fatalf("installed files mismatch: %+v", pkg.InstalledFiles)
	}
}
