package registry

import (
	"context"
	"errors"
)

type PackageKind string

const (
	KindSkill             PackageKind = "skill"
	KindOverlay           PackageKind = "overlay"
	KindWorkspaceTemplate PackageKind = "workspace-template"
	KindAgent             PackageKind = "agent"
)

var ErrPackageNotFound = errors.New("package not found")

type AccessLevel string

const (
	AccessPublic  AccessLevel = "public"
	AccessPrivate AccessLevel = "private"
)

type Dependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint,omitempty"`
}

type OptionSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
}

type ManagedFile struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	CreateIfMissing bool   `json:"createIfMissing,omitempty"`
}

type Skill struct {
	Name  string            `json:"name"`
	Files map[string]string `json:"files"`
}

type PackageVersion struct {
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Kind           PackageKind       `json:"kind"`
	Dependencies   []Dependency      `json:"dependencies,omitempty"`
	Integrity      string            `json:"integrity"`
	ResolvedURL    string            `json:"resolvedURL"`
	Files          map[string]string `json:"files,omitempty"`
	WorkspaceFiles map[string]string `json:"workspaceFiles,omitempty"`
	Skills         []Skill           `json:"skills,omitempty"`
	ManagedFiles   []ManagedFile     `json:"managedFiles,omitempty"`
	InstallOptions []OptionSpec      `json:"installOptions,omitempty"`
}

type Client interface {
	Resolve(ctx context.Context, name string, constraint string) (PackageVersion, error)
}

type PublishMetadata struct {
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Kind         PackageKind `json:"kind"`
	Tag          string      `json:"tag"`
	Access       AccessLevel `json:"access"`
	RegistryURL  string      `json:"registryURL,omitempty"`
	Integrity    string      `json:"integrity"`
	SHA256       string      `json:"sha256"`
	FileCount    int         `json:"fileCount"`
	ArchiveBytes int64       `json:"archiveBytes"`
}

type UploadTarget struct {
	ID  string `json:"id"`
	URL string `json:"url,omitempty"`
}

type PublishRequest struct {
	Metadata PublishMetadata `json:"metadata"`
	Artifact []byte          `json:"-"`
	Token    string          `json:"-"`
}

type PublishResponse struct {
	RegistryURL string `json:"registryURL,omitempty"`
	PackageURL  string `json:"packageURL,omitempty"`
	UploadID    string `json:"uploadID,omitempty"`
}

type PublishClient interface {
	CheckVersion(ctx context.Context, name, version string) (bool, error)
	BeginPublish(ctx context.Context, metadata PublishMetadata) (UploadTarget, error)
	UploadArtifact(ctx context.Context, target UploadTarget, artifact []byte) error
	FinalizePublish(ctx context.Context, metadata PublishMetadata) (PublishResponse, error)
}
