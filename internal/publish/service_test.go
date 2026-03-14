package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marian2js/ocpm/internal/manifest"
	"github.com/marian2js/ocpm/internal/registry"
)

func TestValidateManifestRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name string
		file manifest.File
	}{
		{
			name: "missing name",
			file: manifest.File{
				Version: "0.1.0",
				Kind:    registry.KindSkill,
			},
		},
		{
			name: "invalid version",
			file: manifest.File{
				Name:    "@acme/browser-skill",
				Version: "latest",
				Kind:    registry.KindSkill,
			},
		},
		{
			name: "invalid kind",
			file: manifest.File{
				Name:    "@acme/browser-skill",
				Version: "0.1.0",
				Kind:    "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := validateManifest(tt.file); err == nil {
				t.Fatalf("expected validateManifest to reject %+v", tt.file)
			}
		})
	}
}

func TestDefaultSelectionExcludesRuntimeState(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "skills", "browser", "SKILL.md"), "skill\n")
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")
	writeFile(t, filepath.Join(root, "README.md"), "readme\n")
	writeFile(t, filepath.Join(root, "MEMORY.md"), "memory doc\n")
	writeFile(t, filepath.Join(root, "memory", "state.txt"), "state\n")
	writeFile(t, filepath.Join(root, ".env"), "secret\n")
	writeFile(t, filepath.Join(root, "ocpm-lock.json"), "{}\n")

	files, err := selectFiles(root, manifest.MustRead(filepath.Join(root, manifest.FileName)), matcher{})
	if err != nil {
		t.Fatalf("selectFiles returned error: %v", err)
	}

	got := pathsFromSelected(files)
	if !contains(got, manifest.FileName) || !contains(got, "skills/browser/SKILL.md") || !contains(got, "payload/data.txt") {
		t.Fatalf("expected publishable files to be included, got %v", got)
	}
	if contains(got, "MEMORY.md") || contains(got, "memory/state.txt") || contains(got, ".env") || contains(got, "ocpm-lock.json") {
		t.Fatalf("expected runtime state to be excluded, got %v", got)
	}
}

func TestOCPMIgnoreExcludesFilesUnlessExplicitlyAllowlisted(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "keep.txt"), "keep\n")
	writeFile(t, filepath.Join(root, "payload", "skip.txt"), "skip\n")
	writeFile(t, filepath.Join(root, ignoreFileName), "payload/skip.txt\n")

	ignore, err := loadIgnoreFile(root)
	if err != nil {
		t.Fatalf("loadIgnoreFile returned error: %v", err)
	}

	files, err := selectFiles(root, manifest.MustRead(filepath.Join(root, manifest.FileName)), ignore)
	if err != nil {
		t.Fatalf("selectFiles returned error: %v", err)
	}
	if contains(pathsFromSelected(files), "payload/skip.txt") {
		t.Fatalf("expected payload/skip.txt to be ignored")
	}

	manifestFile := manifest.MustRead(filepath.Join(root, manifest.FileName))
	manifestFile.Files = []string{"payload"}
	writePublishManifest(t, root, manifestFile)

	files, err = selectFiles(root, manifestFile, ignore)
	if err != nil {
		t.Fatalf("selectFiles with explicit files returned error: %v", err)
	}
	if !contains(pathsFromSelected(files), "payload/skip.txt") {
		t.Fatalf("expected explicit files allowlist to override .ocpmignore")
	}
}

func TestArchiveIsDeterministicAcrossMtimeChanges(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	payloadPath := filepath.Join(root, "payload", "data.txt")
	writeFile(t, payloadPath, "payload\n")

	service := NewService(registry.NewMemoryRegistry(nil))
	service.Now = func() time.Time { return time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC) }

	first, err := service.prepare(context.Background(), Request{Cwd: root})
	if err != nil {
		t.Fatalf("prepare returned error: %v", err)
	}

	newTime := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(payloadPath, newTime, newTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	second, err := service.prepare(context.Background(), Request{Cwd: root})
	if err != nil {
		t.Fatalf("second prepare returned error: %v", err)
	}

	if !bytes.Equal(first.archive, second.archive) {
		t.Fatalf("expected deterministic archive output")
	}
}

func TestIntegrityMatchesArchiveBytes(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")

	service := NewService(registry.NewMemoryRegistry(nil))
	prepared, err := service.prepare(context.Background(), Request{Cwd: root})
	if err != nil {
		t.Fatalf("prepare returned error: %v", err)
	}

	sum := sha256.Sum256(prepared.archive)
	want := hex.EncodeToString(sum[:])
	if prepared.result.SHA256 != want {
		t.Fatalf("sha256 mismatch: got %s want %s", prepared.result.SHA256, want)
	}
	if prepared.result.Integrity != "sha256:"+want {
		t.Fatalf("integrity mismatch: got %s", prepared.result.Integrity)
	}
}

func TestPublishDryRunDoesNotUpload(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")

	registryClient := registry.NewMemoryRegistry(nil)
	service := NewService(registryClient)

	result, err := service.Publish(context.Background(), Request{Cwd: root, DryRun: true})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if result.Uploaded {
		t.Fatalf("expected dry-run publish to avoid upload")
	}
	exists, err := registryClient.CheckVersion(context.Background(), "@acme/browser-skill", "0.1.0")
	if err != nil {
		t.Fatalf("CheckVersion returned error: %v", err)
	}
	if exists {
		t.Fatalf("expected dry-run publish to leave registry unchanged")
	}
}

func TestPublishStoresArtifactInMockRegistry(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")

	registryClient := registry.NewMemoryRegistry(nil)
	service := NewService(registryClient)

	result, err := service.Publish(context.Background(), Request{Cwd: root})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if !result.Uploaded {
		t.Fatalf("expected publish to upload")
	}
	if !strings.Contains(result.PackageURL, "mock://registry/") {
		t.Fatalf("expected mock package URL, got %q", result.PackageURL)
	}
}

func TestPublishRejectsDuplicateVersion(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")

	registryClient := registry.NewMemoryRegistry(nil)
	service := NewService(registryClient)

	if _, err := service.Publish(context.Background(), Request{Cwd: root}); err != nil {
		t.Fatalf("first Publish returned error: %v", err)
	}
	if _, err := service.Publish(context.Background(), Request{Cwd: root}); err == nil || !strings.Contains(err.Error(), "already published") {
		t.Fatalf("expected duplicate version rejection, got %v", err)
	}
}

func TestPublishRejectsLargePublicPackage(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "big.bin"), strings.Repeat("x", publicFailBytes+1))

	service := NewService(registry.NewMemoryRegistry(nil))
	if _, err := service.Pack(context.Background(), Request{Cwd: root}); err == nil || !strings.Contains(err.Error(), "above the") {
		t.Fatalf("expected large package failure, got %v", err)
	}
}

func TestPublishRejectsForbiddenExplicitFiles(t *testing.T) {
	root := t.TempDir()
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
		Files:   []string{".env"},
	})
	writeFile(t, filepath.Join(root, ".env"), "secret\n")
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")

	service := NewService(registry.NewMemoryRegistry(nil))
	if _, err := service.Pack(context.Background(), Request{Cwd: root}); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden explicit files failure, got %v", err)
	}
}

func TestPackWritesArchiveFile(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out.tgz")
	writePublishManifest(t, root, manifest.File{
		Name:    "@acme/browser-skill",
		Version: "0.1.0",
		Kind:    registry.KindSkill,
	})
	writeFile(t, filepath.Join(root, "payload", "data.txt"), "payload\n")

	service := NewService(registry.NewMemoryRegistry(nil))
	result, err := service.Pack(context.Background(), Request{Cwd: root, Out: out})
	if err != nil {
		t.Fatalf("Pack returned error: %v", err)
	}
	if result.ArchivePath != out {
		t.Fatalf("expected archive path %q, got %q", out, result.ArchivePath)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected archive file to exist: %v", err)
	}
}

func writePublishManifest(t *testing.T, root string, file manifest.File) {
	t.Helper()
	if err := manifest.Write(filepath.Join(root, manifest.FileName), file); err != nil {
		t.Fatalf("manifest.Write returned error: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func pathsFromSelected(files []selectedFile) []string {
	result := make([]string, 0, len(files))
	for _, file := range files {
		result = append(result, file.RelPath)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
