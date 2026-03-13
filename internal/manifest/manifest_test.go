package manifest

import (
	"path/filepath"
	"testing"
)

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	want := Default(true)
	want.SetDependency("@acme/browser-skill", "1.0.0")
	want.SetOptions("@acme/browser-skill", map[string]string{"homepage": "https://example.com"})

	if err := Write(path, want); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	if got.Private != want.Private || got.Workspace != want.Workspace {
		t.Fatalf("manifest flags mismatch: got=%+v want=%+v", got, want)
	}
	if got.Dependencies["@acme/browser-skill"] != "1.0.0" {
		t.Fatalf("dependency mismatch: %+v", got.Dependencies)
	}
	if got.Options["@acme/browser-skill"]["homepage"] != "https://example.com" {
		t.Fatalf("options mismatch: %+v", got.Options)
	}
}
