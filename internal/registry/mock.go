package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type MemoryRegistry struct {
	packages  map[string][]PackageVersion
	published map[string]map[string]PublishRecord
	uploads   map[string][]byte
}

type PublishRecord struct {
	Metadata PublishMetadata
	Artifact []byte
}

func NewMemoryRegistry(packages []PackageVersion) *MemoryRegistry {
	index := make(map[string][]PackageVersion)
	for _, pkg := range packages {
		pkg = enrichPackage(pkg)
		index[pkg.Name] = append(index[pkg.Name], pkg)
	}

	for name := range index {
		slices.SortFunc(index[name], func(a, b PackageVersion) int {
			return compareVersions(b.Version, a.Version)
		})
	}

	return &MemoryRegistry{
		packages:  index,
		published: map[string]map[string]PublishRecord{},
		uploads:   map[string][]byte{},
	}
}

func NewFixtureRegistry() *MemoryRegistry {
	return NewMemoryRegistry([]PackageVersion{
		{
			Name:    "@acme/browser-skill",
			Version: "1.0.0",
			Kind:    KindSkill,
			Files: map[string]string{
				"metadata/package.txt": "browser-skill fixture payload\n",
			},
			Skills: []Skill{
				{
					Name: "browser",
					Files: map[string]string{
						"SKILL.md":  "# Browser Skill\n\nUse browser tooling for OpenClaw tasks.\nHomepage: {{option:homepage}}\n",
						"README.md": "This skill was installed by ocpm.\n",
					},
				},
			},
			ManagedFiles: []ManagedFile{
				{
					Path:    "AGENTS.md",
					Content: "## Managed Packages\n- `@acme/browser-skill` adds the `browser` skill and points to {{option:homepage}}.\n",
				},
			},
			InstallOptions: []OptionSpec{
				{
					Name:        "homepage",
					Description: "Default browser landing page",
					Default:     "https://openclaw.dev",
				},
			},
		},
		{
			Name:    "@acme/sales-overlay",
			Version: "1.0.0",
			Kind:    KindOverlay,
			Dependencies: []Dependency{
				{Name: "@acme/browser-skill"},
			},
			Files: map[string]string{
				"metadata/package.txt": "sales-overlay fixture payload\n",
			},
			ManagedFiles: []ManagedFile{
				{
					Path:            "MEMORY.md",
					Content:         "## Sales Overlay\n- Prioritize pipeline health and demo follow-up notes.\n",
					CreateIfMissing: true,
				},
			},
		},
		{
			Name:    "@acme/founder-agent",
			Version: "1.0.0",
			Kind:    KindAgent,
			Dependencies: []Dependency{
				{Name: "@acme/browser-skill"},
			},
			Files: map[string]string{
				"metadata/package.txt": "founder-agent fixture payload\n",
			},
			WorkspaceFiles: map[string]string{
				"AGENTS.md":   "# Founder Workspace\n\nThis workspace was created by `ocpm create @acme/founder-agent`.\n",
				"SOUL.md":     "# Soul\n\nOperate like a deliberate founder-operator.\n",
				"TOOLS.md":    "# Tools\n\nUse the installed skills carefully.\n",
				"IDENTITY.md": "# Identity\n\nYou are the founder agent for this workspace.\n",
			},
			Skills: []Skill{
				{
					Name: "founder",
					Files: map[string]string{
						"SKILL.md": "# Founder Skill\n\nCoordinate planning, execution, and feedback loops.\n",
					},
				},
			},
		},
		{
			Name:    "@acme/support-template",
			Version: "1.0.0",
			Kind:    KindWorkspaceTemplate,
			Files: map[string]string{
				"metadata/package.txt": "support-template fixture payload\n",
			},
			WorkspaceFiles: map[string]string{
				"AGENTS.md":   "# Support Workspace\n\nThis workspace was created from the support template.\n",
				"SOUL.md":     "# Soul\n\nOptimize for fast, careful user support.\n",
				"TOOLS.md":    "# Tools\n\nEscalate only when needed.\n",
				"IDENTITY.md": "# Identity\n\nYou are the support workspace template.\n",
				"MEMORY.md":   "# Memory\n\nStore stable support heuristics here.\n",
			},
		},
	})
}

func (r *MemoryRegistry) Resolve(_ context.Context, name string, constraint string) (PackageVersion, error) {
	versions, ok := r.packages[name]
	if !ok {
		return PackageVersion{}, fmt.Errorf("%w: %s", ErrPackageNotFound, name)
	}

	if constraint == "" {
		return versions[0], nil
	}

	for _, pkg := range versions {
		if pkg.Version == constraint {
			return pkg, nil
		}
	}

	return PackageVersion{}, fmt.Errorf("%w: %s@%s", ErrPackageNotFound, name, constraint)
}

func (r *MemoryRegistry) CheckVersion(_ context.Context, name, version string) (bool, error) {
	if versions, ok := r.published[name]; ok {
		_, ok = versions[version]
		return ok, nil
	}
	return false, nil
}

func (r *MemoryRegistry) BeginPublish(_ context.Context, metadata PublishMetadata) (UploadTarget, error) {
	if exists, _ := r.CheckVersion(context.Background(), metadata.Name, metadata.Version); exists {
		return UploadTarget{}, fmt.Errorf("package version already published: %s@%s", metadata.Name, metadata.Version)
	}
	uploadID := fmt.Sprintf("%s@%s", metadata.Name, metadata.Version)
	return UploadTarget{
		ID:  uploadID,
		URL: fmt.Sprintf("mock://upload/%s", strings.TrimPrefix(uploadID, "@")),
	}, nil
}

func (r *MemoryRegistry) UploadArtifact(_ context.Context, target UploadTarget, artifact []byte) error {
	copyArtifact := make([]byte, len(artifact))
	copy(copyArtifact, artifact)
	r.uploads[target.ID] = copyArtifact
	return nil
}

func (r *MemoryRegistry) FinalizePublish(_ context.Context, metadata PublishMetadata) (PublishResponse, error) {
	if exists, _ := r.CheckVersion(context.Background(), metadata.Name, metadata.Version); exists {
		return PublishResponse{}, fmt.Errorf("package version already published: %s@%s", metadata.Name, metadata.Version)
	}

	uploadID := fmt.Sprintf("%s@%s", metadata.Name, metadata.Version)
	artifact, ok := r.uploads[uploadID]
	if !ok {
		return PublishResponse{}, fmt.Errorf("missing uploaded artifact for %s", uploadID)
	}

	if r.published[metadata.Name] == nil {
		r.published[metadata.Name] = map[string]PublishRecord{}
	}
	r.published[metadata.Name][metadata.Version] = PublishRecord{
		Metadata: metadata,
		Artifact: artifact,
	}
	delete(r.uploads, uploadID)

	return PublishResponse{
		RegistryURL: metadata.RegistryURL,
		PackageURL:  fmt.Sprintf("mock://registry/%s/%s", strings.TrimPrefix(metadata.Name, "@"), metadata.Version),
		UploadID:    uploadID,
	}, nil
}

func enrichPackage(pkg PackageVersion) PackageVersion {
	if pkg.ResolvedURL == "" {
		pkg.ResolvedURL = fmt.Sprintf("mock://registry/%s/%s", strings.TrimPrefix(pkg.Name, "@"), pkg.Version)
	}
	if pkg.Integrity == "" {
		integrity, err := ComputePackageIntegrity(pkg)
		if err != nil {
			panic(err)
		}
		pkg.Integrity = integrity
	}
	return pkg
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	limit := len(leftParts)
	if len(rightParts) > limit {
		limit = len(rightParts)
	}

	for i := 0; i < limit; i++ {
		lv := versionPart(leftParts, i)
		rv := versionPart(rightParts, i)
		switch {
		case lv < rv:
			return -1
		case lv > rv:
			return 1
		}
	}

	return 0
}

func versionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(parts[index])
	if err != nil {
		return 0
	}
	return value
}
