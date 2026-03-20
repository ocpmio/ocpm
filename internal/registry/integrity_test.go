package registry

import (
	"errors"
	"testing"
)

func TestComputeAndVerifyPackageIntegrity(t *testing.T) {
	pkg := PackageVersion{
		Name:        "@acme/browser-skill",
		Version:     "1.0.0",
		Kind:        KindSkill,
		ResolvedURL: "mock://registry/acme/browser-skill/1.0.0",
		Dependencies: []Dependency{
			{Name: "@acme/core", Constraint: "1.0.0"},
		},
		Files: map[string]string{
			"metadata/package.txt": "payload\n",
		},
		ManagedFiles: []ManagedFile{
			{Path: "AGENTS.md", Content: "managed\n"},
		},
		InstallOptions: []OptionSpec{
			{Name: "homepage", Default: "https://openclaw.dev"},
		},
		Skills: []Skill{
			{
				Name: "browser",
				Files: map[string]string{
					"README.md": "skill docs\n",
					"SKILL.md":  "# Browser\n",
				},
			},
		},
	}

	integrity, err := ComputePackageIntegrity(pkg)
	if err != nil {
		t.Fatalf("ComputePackageIntegrity returned error: %v", err)
	}

	pkg.Integrity = integrity
	if err := VerifyPackage(pkg); err != nil {
		t.Fatalf("VerifyPackage returned error: %v", err)
	}
}

func TestVerifyPackageRejectsTamperedContent(t *testing.T) {
	pkg := PackageVersion{
		Name:        "@acme/browser-skill",
		Version:     "1.0.0",
		Kind:        KindSkill,
		ResolvedURL: "mock://registry/acme/browser-skill/1.0.0",
		Files: map[string]string{
			"metadata/package.txt": "payload\n",
		},
	}

	integrity, err := ComputePackageIntegrity(pkg)
	if err != nil {
		t.Fatalf("ComputePackageIntegrity returned error: %v", err)
	}

	pkg.Integrity = integrity
	pkg.Files["metadata/package.txt"] = "tampered\n"

	err = VerifyPackage(pkg)
	if err == nil {
		t.Fatalf("expected integrity mismatch")
	}
	if !errors.Is(err, ErrIntegrityMismatch) {
		t.Fatalf("expected ErrIntegrityMismatch, got %v", err)
	}
}

func TestVerifyPackageRejectsMissingAndUnsupportedIntegrity(t *testing.T) {
	tests := []struct {
		name      string
		integrity string
		wantErr   error
	}{
		{name: "missing", integrity: "", wantErr: ErrIntegrityMissing},
		{name: "unsupported", integrity: "sha512:abc", wantErr: ErrIntegrityUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := VerifyPackage(PackageVersion{
				Name:        "@acme/browser-skill",
				Version:     "1.0.0",
				Kind:        KindSkill,
				ResolvedURL: "mock://registry/acme/browser-skill/1.0.0",
				Integrity:   test.integrity,
			})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("expected %v, got %v", test.wantErr, err)
			}
		})
	}
}
