package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/marian2js/ocpm/internal/config"
	"github.com/marian2js/ocpm/internal/manifest"
	"github.com/marian2js/ocpm/internal/registry"
)

const (
	ignoreFileName       = ".ocpmignore"
	publicWarnBytes      = 5 * 1024 * 1024
	publicFailBytes      = 10 * 1024 * 1024
	hardFailBytes        = 20 * 1024 * 1024
	normalizedPermission = 0o644
	execPermission       = 0o755
)

type Service struct {
	Registry registry.PublishClient
	Now      func() time.Time
}

type Request struct {
	Cwd         string
	Path        string
	DryRun      bool
	Private     bool
	Tag         string
	RegistryURL string
	Out         string
	Access      string
	Yes         bool
	ListFiles   bool
}

type Result struct {
	Name              string               `json:"name"`
	Version           string               `json:"version"`
	Kind              registry.PackageKind `json:"kind"`
	Tag               string               `json:"tag"`
	Access            registry.AccessLevel `json:"access"`
	RegistryURL       string               `json:"registryURL,omitempty"`
	PackageURL        string               `json:"packageURL,omitempty"`
	ArchivePath       string               `json:"archivePath,omitempty"`
	SHA256            string               `json:"sha256"`
	Integrity         string               `json:"integrity"`
	FileCount         int                  `json:"fileCount"`
	ArchiveBytes      int64                `json:"archiveBytes"`
	UncompressedBytes int64                `json:"uncompressedBytes"`
	DryRun            bool                 `json:"dryRun"`
	Uploaded          bool                 `json:"uploaded"`
	Packed            bool                 `json:"packed"`
	Files             []PackagedFile       `json:"files,omitempty"`
	Warnings          []string             `json:"warnings,omitempty"`
	Manifest          ManifestSummary      `json:"manifest"`
}

type PackagedFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type ManifestSummary struct {
	Description string   `json:"description,omitempty"`
	License     string   `json:"license,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type selectedFile struct {
	RelPath string
	Bytes   []byte
	Mode    fs.FileMode
	SHA256  string
}

func NewService(client registry.PublishClient) *Service {
	return &Service{
		Registry: client,
		Now:      time.Now,
	}
}

func (s *Service) Pack(ctx context.Context, request Request) (Result, error) {
	prepared, err := s.prepare(ctx, request)
	if err != nil {
		return Result{}, err
	}

	result := prepared.result
	result.Packed = true
	if request.DryRun {
		return result, nil
	}

	outputPath := request.Out
	if outputPath == "" {
		outputPath = filepath.Join(prepared.root, defaultArchiveName(result.Name, result.Version))
	}
	if err := writeTarball(outputPath, prepared.archive); err != nil {
		return Result{}, err
	}
	result.ArchivePath = outputPath
	return result, nil
}

func (s *Service) Publish(ctx context.Context, request Request) (Result, error) {
	prepared, err := s.prepare(ctx, request)
	if err != nil {
		return Result{}, err
	}

	result := prepared.result
	if request.Out != "" {
		result.Packed = true
		if request.DryRun {
			result.ArchivePath = request.Out
			return result, nil
		}
		if err := writeTarball(request.Out, prepared.archive); err != nil {
			return Result{}, err
		}
		result.ArchivePath = request.Out
		return result, nil
	}

	if request.DryRun {
		return result, nil
	}

	if !request.Yes {
		return Result{}, fmt.Errorf("refusing to upload without --yes")
	}

	exists, err := s.Registry.CheckVersion(ctx, result.Name, result.Version)
	if err != nil {
		return Result{}, err
	}
	if exists {
		return Result{}, fmt.Errorf("package version already published: %s@%s", result.Name, result.Version)
	}

	metadata := registry.PublishMetadata{
		Name:         result.Name,
		Version:      result.Version,
		Kind:         result.Kind,
		Tag:          result.Tag,
		Access:       result.Access,
		RegistryURL:  result.RegistryURL,
		Integrity:    result.Integrity,
		SHA256:       result.SHA256,
		FileCount:    result.FileCount,
		ArchiveBytes: result.ArchiveBytes,
	}
	target, err := s.Registry.BeginPublish(ctx, metadata)
	if err != nil {
		return Result{}, err
	}
	if err := s.Registry.UploadArtifact(ctx, target, prepared.archive); err != nil {
		return Result{}, err
	}
	response, err := s.Registry.FinalizePublish(ctx, metadata)
	if err != nil {
		return Result{}, err
	}

	result.Uploaded = true
	result.PackageURL = response.PackageURL
	if response.RegistryURL != "" {
		result.RegistryURL = response.RegistryURL
	}
	return result, nil
}

type preparedPackage struct {
	root     string
	manifest manifest.File
	files    []selectedFile
	archive  []byte
	result   Result
}

func (s *Service) prepare(_ context.Context, request Request) (preparedPackage, error) {
	root := request.Path
	if root == "" {
		root = request.Cwd
	}
	root = filepath.Clean(root)

	manifestFile, err := manifest.ReadFromDir(root)
	if err != nil {
		return preparedPackage{}, err
	}

	warnings, err := validateManifest(manifestFile)
	if err != nil {
		return preparedPackage{}, err
	}

	settings := config.ResolveRegistrySettings(request.RegistryURL, manifestRegistryURL(manifestFile))
	tag := request.Tag
	if tag == "" {
		tag = manifestTag(manifestFile)
	}
	if tag == "" {
		tag = "latest"
	}

	access, err := resolveAccess(manifestFile, request.Access, request.Private)
	if err != nil {
		return preparedPackage{}, err
	}

	ignore, err := loadIgnoreFile(root)
	if err != nil {
		return preparedPackage{}, err
	}

	files, err := selectFiles(root, manifestFile, ignore)
	if err != nil {
		return preparedPackage{}, err
	}

	validationWarnings, err := validateSelectedFiles(root, manifestFile, files, access)
	if err != nil {
		return preparedPackage{}, err
	}
	warnings = append(warnings, validationWarnings...)

	archive, archiveBytes, err := buildArchive(manifestFile, files)
	if err != nil {
		return preparedPackage{}, err
	}

	uncompressed := int64(0)
	packagedFiles := make([]PackagedFile, 0, len(files))
	for _, file := range files {
		uncompressed += int64(len(file.Bytes))
		packagedFiles = append(packagedFiles, PackagedFile{
			Path:   file.RelPath,
			Bytes:  int64(len(file.Bytes)),
			SHA256: file.SHA256,
		})
	}

	sum := sha256.Sum256(archive)
	sha := hex.EncodeToString(sum[:])

	result := Result{
		Name:              manifestFile.Name,
		Version:           manifestFile.Version,
		Kind:              manifestFile.Kind,
		Tag:               tag,
		Access:            access,
		RegistryURL:       settings.URL,
		SHA256:            sha,
		Integrity:         "sha256:" + sha,
		FileCount:         len(files),
		ArchiveBytes:      archiveBytes,
		UncompressedBytes: uncompressed,
		DryRun:            request.DryRun,
		Files:             packagedFiles,
		Warnings:          warnings,
		Manifest: ManifestSummary{
			Description: manifestFile.Description,
			License:     effectiveLicense(root, manifestFile),
			Homepage:    manifestFile.Homepage,
			Repository:  manifestFile.Repository,
			Keywords:    append([]string(nil), manifestFile.Keywords...),
		},
	}

	slices.Sort(result.Warnings)
	slices.SortFunc(result.Files, func(a, b PackagedFile) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})

	return preparedPackage{
		root:     root,
		manifest: manifestFile,
		files:    files,
		archive:  archive,
		result:   result,
	}, nil
}

func validateManifest(file manifest.File) ([]string, error) {
	var warnings []string
	if !validPackageName(file.Name) {
		return nil, fmt.Errorf("ocpm.json must define a valid package name")
	}
	if !validSemver(file.Version) {
		return nil, fmt.Errorf("ocpm.json must define a valid semver version")
	}
	if !validKind(file.Kind) {
		return nil, fmt.Errorf("ocpm.json must define a valid package kind")
	}
	if file.PublishConfig != nil && file.PublishConfig.Access != "" && file.PublishConfig.Access != string(registry.AccessPublic) && file.PublishConfig.Access != string(registry.AccessPrivate) {
		return nil, fmt.Errorf("publishConfig.access must be public or private")
	}
	for _, entry := range file.Files {
		if err := validateRelativePath(entry); err != nil {
			return nil, fmt.Errorf("invalid files entry %q: %w", entry, err)
		}
	}
	if file.License == "" {
		warnings = append(warnings, "license is not set in ocpm.json; publish output will fall back to a detected Apache-2.0 LICENSE file when possible")
	}
	return warnings, nil
}

func selectFiles(root string, manifestFile manifest.File, ignore matcher) ([]selectedFile, error) {
	seen := map[string]struct{}{}
	var collected []selectedFile

	addPath := func(relPath string, explicit bool) error {
		relPath = filepath.ToSlash(filepath.Clean(relPath))
		if relPath == "." || relPath == "" {
			return fmt.Errorf("root path is not publishable; enumerate files or directories explicitly")
		}
		if err := validateRelativePath(relPath); err != nil {
			return err
		}
		if hardExcluded(relPath) {
			if explicit {
				return fmt.Errorf("%s is forbidden from published artifacts", relPath)
			}
			return nil
		}

		absolute := filepath.Join(root, relPath)
		info, err := os.Lstat(absolute)
		if err != nil {
			if os.IsNotExist(err) && !explicit {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; symlinks are not supported", relPath)
		}
		if info.IsDir() {
			return filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				relative = filepath.ToSlash(relative)
				if relative == "." {
					return nil
				}
				if hardExcluded(relative) {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("%s is a symlink; symlinks are not supported", relative)
				}
				if !explicit && ignore.matches(relative, entry.IsDir()) {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.IsDir() {
					return nil
				}
				return includeFile(root, relative, &collected, seen)
			})
		}

		if !explicit && ignore.matches(relPath, false) {
			return nil
		}
		return includeFile(root, relPath, &collected, seen)
	}

	if len(manifestFile.Files) > 0 {
		if err := addPath(manifest.FileName, true); err != nil {
			return nil, err
		}
		for _, entry := range manifestFile.Files {
			if entry == manifest.FileName {
				continue
			}
			if err := addPath(entry, true); err != nil {
				return nil, err
			}
		}
	} else {
		for _, entry := range defaultPublishEntries() {
			if err := addPath(entry, false); err != nil {
				return nil, err
			}
		}
	}

	slices.SortFunc(collected, func(a, b selectedFile) int {
		if a.RelPath < b.RelPath {
			return -1
		}
		if a.RelPath > b.RelPath {
			return 1
		}
		return 0
	})
	return collected, nil
}

func includeFile(root, relPath string, collected *[]selectedFile, seen map[string]struct{}) error {
	relPath = filepath.ToSlash(relPath)
	if _, ok := seen[relPath]; ok {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(root, relPath))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	*collected = append(*collected, selectedFile{
		RelPath: relPath,
		Bytes:   data,
		Mode:    info.Mode(),
		SHA256:  hex.EncodeToString(sum[:]),
	})
	seen[relPath] = struct{}{}
	return nil
}

func validateSelectedFiles(root string, manifestFile manifest.File, files []selectedFile, access registry.AccessLevel) ([]string, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no publishable files were selected")
	}
	if len(files) == 1 && files[0].RelPath == manifest.FileName {
		return nil, fmt.Errorf("package is empty; include at least one publishable file beyond ocpm.json")
	}

	total := int64(0)
	for _, file := range files {
		total += int64(len(file.Bytes))
	}

	var warnings []string
	if total > hardFailBytes {
		return nil, fmt.Errorf("package contents are %d bytes, above the hard cap of %d bytes", total, hardFailBytes)
	}
	if access == registry.AccessPublic && total > publicFailBytes {
		return nil, fmt.Errorf("public package contents are %d bytes, above the %d byte limit", total, publicFailBytes)
	}
	if total > publicWarnBytes {
		warnings = append(warnings, fmt.Sprintf("package contents are %d bytes; artifacts above %d bytes are discouraged", total, publicWarnBytes))
	}

	for _, path := range metadataReferences(manifestFile) {
		if !containsPath(files, path) {
			return nil, fmt.Errorf("ocpm metadata references %s, but it is not included in the publish set", path)
		}
	}

	_ = root
	return warnings, nil
}

func buildArchive(manifestFile manifest.File, files []selectedFile) ([]byte, int64, error) {
	var archive bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&archive, gzip.BestCompression)
	if err != nil {
		return nil, 0, err
	}
	gzipWriter.Name = ""
	gzipWriter.ModTime = time.Unix(0, 0)
	gzipWriter.OS = 255

	tarWriter := tar.NewWriter(gzipWriter)
	for _, file := range files {
		header := &tar.Header{
			Name:    filepath.ToSlash(filepath.Join("package", file.RelPath)),
			Mode:    normalizedMode(file.Mode),
			Size:    int64(len(file.Bytes)),
			ModTime: time.Unix(0, 0),
			Format:  tar.FormatPAX,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, 0, err
		}
		if _, err := io.Copy(tarWriter, bytes.NewReader(file.Bytes)); err != nil {
			return nil, 0, err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, 0, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, 0, err
	}
	_ = manifestFile
	return archive.Bytes(), int64(archive.Len()), nil
}

func writeTarball(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func normalizedMode(mode fs.FileMode) int64 {
	if mode&0o111 != 0 {
		return execPermission
	}
	return normalizedPermission
}

func defaultArchiveName(name, version string) string {
	sanitized := strings.TrimPrefix(name, "@")
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	return fmt.Sprintf("%s-%s.tgz", sanitized, version)
}

func resolveAccess(manifestFile manifest.File, flagAccess string, private bool) (registry.AccessLevel, error) {
	if private {
		return registry.AccessPrivate, nil
	}
	if flagAccess != "" {
		switch flagAccess {
		case string(registry.AccessPublic):
			return registry.AccessPublic, nil
		case string(registry.AccessPrivate):
			return registry.AccessPrivate, nil
		default:
			return "", fmt.Errorf("access must be public or private")
		}
	}
	if manifestFile.PublishConfig != nil && manifestFile.PublishConfig.Access != "" {
		switch manifestFile.PublishConfig.Access {
		case string(registry.AccessPublic):
			return registry.AccessPublic, nil
		case string(registry.AccessPrivate):
			return registry.AccessPrivate, nil
		default:
			return "", fmt.Errorf("publishConfig.access must be public or private")
		}
	}
	return registry.AccessPublic, nil
}

func effectiveLicense(root string, manifestFile manifest.File) string {
	if manifestFile.License != "" {
		return manifestFile.License
	}
	data, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err == nil && bytes.Contains(data, []byte("Apache License")) {
		return "Apache-2.0"
	}
	return ""
}

func manifestRegistryURL(file manifest.File) string {
	if file.PublishConfig == nil {
		return ""
	}
	return file.PublishConfig.Registry
}

func manifestTag(file manifest.File) string {
	if file.PublishConfig == nil {
		return ""
	}
	return file.PublishConfig.Tag
}

func defaultPublishEntries() []string {
	return []string{
		manifest.FileName,
		"AGENTS.md",
		"SOUL.md",
		"IDENTITY.md",
		"TOOLS.md",
		"MEMORY.md",
		"BOOTSTRAP.md",
		"USER.md",
		"HEARTBEAT.md",
		"README",
		"README.md",
		"README.txt",
		"LICENSE",
		"LICENSE.md",
		"LICENSE.txt",
		"skills",
		"templates",
		"payload",
	}
}

func metadataReferences(file manifest.File) []string {
	if file.OCPM == nil {
		return nil
	}
	var refs []string
	refs = append(refs, file.OCPM.Skills...)
	refs = append(refs, file.OCPM.Templates...)
	refs = append(refs, file.OCPM.Payload...)
	return refs
}

func containsPath(files []selectedFile, target string) bool {
	target = filepath.ToSlash(filepath.Clean(target))
	for _, file := range files {
		if file.RelPath == target || strings.HasPrefix(file.RelPath, target+"/") {
			return true
		}
	}
	return false
}

func validKind(kind registry.PackageKind) bool {
	switch kind {
	case registry.KindSkill, registry.KindOverlay, registry.KindWorkspaceTemplate, registry.KindAgent:
		return true
	default:
		return false
	}
}

func validateRelativePath(path string) error {
	path = filepath.ToSlash(path)
	switch {
	case path == "":
		return fmt.Errorf("path is empty")
	case strings.HasPrefix(path, "/"):
		return fmt.Errorf("absolute paths are not allowed")
	case strings.HasPrefix(path, "../") || strings.Contains(path, "/../") || path == "..":
		return fmt.Errorf("path traversal is not allowed")
	default:
		return nil
	}
}

func validPackageName(name string) bool {
	if name == "" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(name, "@"), "/")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				continue
			}
			return false
		}
	}
	return true
}

func validSemver(version string) bool {
	if version == "" {
		return false
	}
	parts := strings.SplitN(version, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return false
	}
	for _, part := range core {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
