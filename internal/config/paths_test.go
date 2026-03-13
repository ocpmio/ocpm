package config

import (
	"path/filepath"
	"testing"
)

func TestResolveIncludesAppName(t *testing.T) {
	paths, err := Resolve("ocpm-test")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if filepath.Base(paths.ConfigDir) != "ocpm-test" {
		t.Fatalf("expected config dir to include app name, got %q", paths.ConfigDir)
	}

	if filepath.Base(paths.CacheDir) != "ocpm-test" {
		t.Fatalf("expected cache dir to include app name, got %q", paths.CacheDir)
	}

	if filepath.Base(paths.StateDir) != "ocpm-test" && filepath.Base(filepath.Dir(paths.StateDir)) != "ocpm-test" {
		t.Fatalf("expected state dir to include app name, got %q", paths.StateDir)
	}
}
