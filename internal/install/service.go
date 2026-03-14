package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/marian2js/ocpm/internal/lockfile"
	"github.com/marian2js/ocpm/internal/managedsections"
	"github.com/marian2js/ocpm/internal/manifest"
	"github.com/marian2js/ocpm/internal/materialize"
	"github.com/marian2js/ocpm/internal/openclaw"
	"github.com/marian2js/ocpm/internal/registry"
	"github.com/marian2js/ocpm/internal/ui"
	"github.com/marian2js/ocpm/internal/workspace"
)

type Service struct {
	Registry registry.Client
	Resolver *workspace.Resolver
	OpenClaw openclaw.Client
	Now      func() time.Time
}

type ChangeRequest struct {
	WorkspacePath    string
	Cwd              string
	Package          string
	Version          string
	PackageOptions   map[string]string
	DryRun           bool
	AllowUnsafeKinds bool
	Prompter         ui.Prompter
}

type CreateRequest struct {
	Dir              string
	Cwd              string
	Package          string
	Version          string
	PackageOptions   map[string]string
	DryRun           bool
	RunOpenClawSetup bool
	Prompter         ui.Prompter
}

type InitRequest struct {
	Path              string
	Cwd               string
	WorkspaceManifest bool
	Name              string
	Version           string
	Kind              registry.PackageKind
	Private           bool
	Force             bool
	DryRun            bool
}

type UpdateRequest struct {
	WorkspacePath  string
	Cwd            string
	Package        string
	Version        string
	PackageOptions map[string]string
	DryRun         bool
	Prompter       ui.Prompter
}

type ListRequest struct {
	WorkspacePath string
	Cwd           string
}

type DoctorRequest struct {
	WorkspacePath string
	Cwd           string
}

type ChangeResult struct {
	WorkspacePath   string                  `json:"workspacePath"`
	Package         string                  `json:"package,omitempty"`
	PackageKind     registry.PackageKind    `json:"packageKind,omitempty"`
	WorkspaceSource string                  `json:"workspaceSource,omitempty"`
	Operations      []materialize.Operation `json:"operations"`
	Skipped         []materialize.Operation `json:"skipped,omitempty"`
}

type ListPackage struct {
	Name    string               `json:"name"`
	Version string               `json:"version,omitempty"`
	Kind    registry.PackageKind `json:"kind,omitempty"`
	Status  string               `json:"status"`
}

type ListResult struct {
	WorkspacePath string        `json:"workspacePath"`
	Packages      []ListPackage `json:"packages"`
}

type DoctorResult struct {
	CurrentPath            string              `json:"currentPath"`
	Workspace              workspace.Detection `json:"workspace"`
	OpenClawInstalled      bool                `json:"openClawInstalled"`
	DefaultWorkspace       string              `json:"defaultWorkspace,omitempty"`
	ManifestExists         bool                `json:"manifestExists"`
	LockfileExists         bool                `json:"lockfileExists"`
	ManifestLockConsistent bool                `json:"manifestLockConsistent"`
	CorruptedManagedFiles  []string            `json:"corruptedManagedFiles,omitempty"`
	Issues                 []string            `json:"issues,omitempty"`
}

func NewService(registryClient registry.Client, resolver *workspace.Resolver, openclawClient openclaw.Client) *Service {
	return &Service{
		Registry: registryClient,
		Resolver: resolver,
		OpenClaw: openclawClient,
		Now:      time.Now,
	}
}

func (s *Service) Init(_ context.Context, request InitRequest) (ChangeResult, error) {
	target := request.Path
	if target == "" {
		target = request.Cwd
	}
	target = filepath.Clean(target)

	workspaceManifest := request.WorkspaceManifest
	if detection, err := workspace.Detect(target); err == nil && detection.LooksLikeWorkspace {
		workspaceManifest = true
	}

	manifestPath := filepath.Join(target, manifest.FileName)
	if !request.Force {
		if _, err := os.Stat(manifestPath); err == nil {
			return ChangeResult{}, fmt.Errorf("%s already exists; use --force to overwrite it", manifest.FileName)
		}
	}

	file, err := defaultInitManifest(target, workspaceManifest, request)
	if err != nil {
		return ChangeResult{}, err
	}
	data, err := manifest.Marshal(file)
	if err != nil {
		return ChangeResult{}, err
	}

	result := ChangeResult{
		WorkspacePath: target,
		Operations: []materialize.Operation{
			{Action: "write-file", Path: manifest.FileName},
		},
	}

	if workspaceManifest {
		result.Operations = append(result.Operations, materialize.Operation{Action: "ensure-dir", Path: ".ocpm"})
	}

	if request.DryRun {
		return result, nil
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return ChangeResult{}, err
	}
	if err := writeIfChanged(manifestPath, data); err != nil {
		return ChangeResult{}, err
	}
	if workspaceManifest {
		if err := os.MkdirAll(filepath.Join(target, ".ocpm"), 0o755); err != nil {
			return ChangeResult{}, err
		}
	}
	return result, nil
}

func (s *Service) Add(ctx context.Context, request ChangeRequest) (ChangeResult, error) {
	resolution, err := s.Resolver.Resolve(ctx, request.Cwd, request.WorkspacePath)
	if err != nil {
		return ChangeResult{}, err
	}

	manifestFile, err := readOrDefaultManifest(resolution.Path, true)
	if err != nil {
		return ChangeResult{}, err
	}
	currentLock, _ := readLockIfPresent(resolution.Path)

	manifestFile.SetDependency(request.Package, request.Version)

	resolved, err := s.resolveGraph(ctx, manifestFile.Dependencies)
	if err != nil {
		return ChangeResult{}, err
	}

	targetPackage, ok := resolved[request.Package]
	if !ok {
		return ChangeResult{}, fmt.Errorf("resolved graph did not contain %s", request.Package)
	}
	if !request.AllowUnsafeKinds && !isAddSafeKind(targetPackage.Kind) {
		return ChangeResult{}, fmt.Errorf("%s is a %s package; use ocpm create or pass --allow-unsafe-kind", request.Package, targetPackage.Kind)
	}
	for _, pkg := range resolved {
		if !request.AllowUnsafeKinds && !isAddSafeKind(pkg.Kind) {
			return ChangeResult{}, fmt.Errorf("%s depends on %s (%s), which cannot be added into an existing workspace safely", request.Package, pkg.Name, pkg.Kind)
		}
	}

	desired, topLevelOptions, err := s.desiredPackages(resolved, manifestFile, currentLock, request.Package, request.PackageOptions, request.Prompter)
	if err != nil {
		return ChangeResult{}, err
	}
	for name, options := range topLevelOptions {
		manifestFile.SetOptions(name, options)
	}

	syncResult, err := materialize.Sync(materialize.SyncRequest{
		WorkspacePath: resolution.Path,
		Current:       currentLock,
		Packages:      desired,
		DryRun:        request.DryRun,
		Now:           s.Now(),
	})
	if err != nil {
		return ChangeResult{}, err
	}

	ops, err := s.persistWorkspaceState(resolution.Path, manifestFile, syncResult.Lock, request.DryRun)
	if err != nil {
		return ChangeResult{}, err
	}

	return ChangeResult{
		WorkspacePath:   resolution.Path,
		WorkspaceSource: resolution.Source,
		Package:         request.Package,
		PackageKind:     targetPackage.Kind,
		Operations:      append(syncResult.Operations, ops...),
		Skipped:         syncResult.Skipped,
	}, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (ChangeResult, error) {
	targetDir := request.Dir
	if targetDir == "" {
		targetDir = filepath.Join(request.Cwd, defaultDirName(request.Package))
	}
	targetDir = filepath.Clean(targetDir)

	empty, err := dirEmpty(targetDir)
	if err != nil {
		return ChangeResult{}, err
	}
	if !empty {
		return ChangeResult{}, fmt.Errorf("%s already exists and is not empty", targetDir)
	}

	pkg, err := s.Registry.Resolve(ctx, request.Package, request.Version)
	if err != nil {
		return ChangeResult{}, err
	}
	if !isCreateSafeKind(pkg.Kind) {
		return ChangeResult{}, fmt.Errorf("%s is a %s package; use ocpm add instead", request.Package, pkg.Kind)
	}

	manifestFile := manifest.Default(true)
	manifestFile.SetDependency(request.Package, request.Version)
	resolved, err := s.resolveGraph(ctx, manifestFile.Dependencies)
	if err != nil {
		return ChangeResult{}, err
	}
	desired, topLevelOptions, err := s.desiredPackages(resolved, manifestFile, lockfile.File{}, request.Package, request.PackageOptions, request.Prompter)
	if err != nil {
		return ChangeResult{}, err
	}
	manifestFile.SetOptions(request.Package, topLevelOptions[request.Package])

	syncResult, err := materialize.Sync(materialize.SyncRequest{
		WorkspacePath:         targetDir,
		Packages:              desired,
		DryRun:                request.DryRun,
		AllowDirectRootWrites: true,
		Now:                   s.Now(),
	})
	if err != nil {
		return ChangeResult{}, err
	}

	ops := []materialize.Operation{{Action: "ensure-dir", Path: ".ocpm"}}
	persistedOps, err := s.persistWorkspaceState(targetDir, manifestFile, syncResult.Lock, request.DryRun)
	if err != nil {
		return ChangeResult{}, err
	}
	ops = append(ops, syncResult.Operations...)
	ops = append(ops, persistedOps...)

	if request.RunOpenClawSetup {
		ops = append(ops, materialize.Operation{Action: "openclaw-setup", Path: targetDir})
		if !request.DryRun {
			if err := s.OpenClaw.SetupWorkspace(ctx, targetDir); err != nil {
				return ChangeResult{}, err
			}
		}
	}

	if !request.DryRun {
		if err := os.MkdirAll(filepath.Join(targetDir, ".ocpm"), 0o755); err != nil {
			return ChangeResult{}, err
		}
	}

	return ChangeResult{
		WorkspacePath: targetDir,
		Package:       request.Package,
		PackageKind:   pkg.Kind,
		Operations:    ops,
		Skipped:       syncResult.Skipped,
	}, nil
}

func (s *Service) Remove(ctx context.Context, request ChangeRequest) (ChangeResult, error) {
	resolution, err := s.Resolver.Resolve(ctx, request.Cwd, request.WorkspacePath)
	if err != nil {
		return ChangeResult{}, err
	}

	manifestFile, err := readOrDefaultManifest(resolution.Path, true)
	if err != nil {
		return ChangeResult{}, err
	}
	currentLock, err := readRequiredLock(resolution.Path)
	if err != nil {
		return ChangeResult{}, err
	}

	if _, ok := manifestFile.Dependencies[request.Package]; !ok {
		if _, ok := currentLock.FindPackage(request.Package); ok {
			return ChangeResult{}, fmt.Errorf("%s is installed as a dependency; remove the top-level package that requires it", request.Package)
		}
		return ChangeResult{}, fmt.Errorf("%s is not installed in %s", request.Package, resolution.Path)
	}

	manifestFile.RemoveDependency(request.Package)
	resolved, err := s.resolveGraph(ctx, manifestFile.Dependencies)
	if err != nil {
		return ChangeResult{}, err
	}

	desired, topLevelOptions, err := s.desiredPackages(resolved, manifestFile, currentLock, "", nil, request.Prompter)
	if err != nil {
		return ChangeResult{}, err
	}
	manifestFile.Options = map[string]map[string]string{}
	for name, options := range topLevelOptions {
		manifestFile.SetOptions(name, options)
	}

	syncResult, err := materialize.Sync(materialize.SyncRequest{
		WorkspacePath: resolution.Path,
		Current:       currentLock,
		Packages:      desired,
		DryRun:        request.DryRun,
		Now:           s.Now(),
	})
	if err != nil {
		return ChangeResult{}, err
	}

	ops, err := s.persistWorkspaceState(resolution.Path, manifestFile, syncResult.Lock, request.DryRun)
	if err != nil {
		return ChangeResult{}, err
	}

	return ChangeResult{
		WorkspacePath:   resolution.Path,
		WorkspaceSource: resolution.Source,
		Package:         request.Package,
		Operations:      append(syncResult.Operations, ops...),
		Skipped:         syncResult.Skipped,
	}, nil
}

func (s *Service) Update(ctx context.Context, request UpdateRequest) (ChangeResult, error) {
	resolution, err := s.Resolver.Resolve(ctx, request.Cwd, request.WorkspacePath)
	if err != nil {
		return ChangeResult{}, err
	}

	manifestFile, err := readOrDefaultManifest(resolution.Path, true)
	if err != nil {
		return ChangeResult{}, err
	}
	currentLock, _ := readLockIfPresent(resolution.Path)

	if request.Package != "" {
		if _, ok := manifestFile.Dependencies[request.Package]; !ok {
			return ChangeResult{}, fmt.Errorf("%s is not a top-level dependency in %s", request.Package, resolution.Path)
		}
		manifestFile.SetDependency(request.Package, request.Version)
	}

	resolved, err := s.resolveGraph(ctx, manifestFile.Dependencies)
	if err != nil {
		return ChangeResult{}, err
	}

	desired, topLevelOptions, err := s.desiredPackages(resolved, manifestFile, currentLock, request.Package, request.PackageOptions, request.Prompter)
	if err != nil {
		return ChangeResult{}, err
	}
	manifestFile.Options = map[string]map[string]string{}
	for name, options := range topLevelOptions {
		manifestFile.SetOptions(name, options)
	}

	syncResult, err := materialize.Sync(materialize.SyncRequest{
		WorkspacePath: resolution.Path,
		Current:       currentLock,
		Packages:      desired,
		DryRun:        request.DryRun,
		Now:           s.Now(),
	})
	if err != nil {
		return ChangeResult{}, err
	}

	ops, err := s.persistWorkspaceState(resolution.Path, manifestFile, syncResult.Lock, request.DryRun)
	if err != nil {
		return ChangeResult{}, err
	}

	kind := registry.PackageKind("")
	if request.Package != "" {
		kind = resolved[request.Package].Kind
	}
	return ChangeResult{
		WorkspacePath:   resolution.Path,
		WorkspaceSource: resolution.Source,
		Package:         request.Package,
		PackageKind:     kind,
		Operations:      append(syncResult.Operations, ops...),
		Skipped:         syncResult.Skipped,
	}, nil
}

func (s *Service) List(ctx context.Context, request ListRequest) (ListResult, error) {
	resolution, err := s.Resolver.Resolve(ctx, request.Cwd, request.WorkspacePath)
	if err != nil {
		return ListResult{}, err
	}

	manifestFile, _ := readManifestIfPresent(resolution.Path)
	lockFile, _ := readLockIfPresent(resolution.Path)

	items := map[string]ListPackage{}
	for _, pkg := range lockFile.Packages {
		status := "dependency"
		if _, ok := manifestFile.Dependencies[pkg.Name]; ok {
			status = "installed"
		}
		items[pkg.Name] = ListPackage{
			Name:    pkg.Name,
			Version: pkg.Version,
			Kind:    pkg.Kind,
			Status:  status,
		}
	}

	for name := range manifestFile.Dependencies {
		if _, ok := items[name]; ok {
			continue
		}
		items[name] = ListPackage{Name: name, Status: "manifest-only"}
	}

	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	slices.Sort(names)

	result := ListResult{WorkspacePath: resolution.Path}
	for _, name := range names {
		result.Packages = append(result.Packages, items[name])
	}
	return result, nil
}

func (s *Service) Doctor(ctx context.Context, request DoctorRequest) (DoctorResult, error) {
	target := request.WorkspacePath
	if target == "" {
		target = request.Cwd
	}
	target = filepath.Clean(target)

	detection, err := workspace.Detect(target)
	if err != nil {
		return DoctorResult{}, err
	}

	result := DoctorResult{
		CurrentPath:            target,
		Workspace:              detection,
		OpenClawInstalled:      s.OpenClaw != nil && s.OpenClaw.IsInstalled(ctx),
		ManifestExists:         manifest.Exists(target),
		LockfileExists:         lockfile.Exists(target),
		ManifestLockConsistent: true,
	}

	if result.OpenClawInstalled {
		defaultWorkspace, err := s.OpenClaw.DefaultWorkspace(ctx)
		if err == nil {
			result.DefaultWorkspace = defaultWorkspace
		}
	}

	manifestFile, manifestErr := readManifestIfPresent(target)
	lockFile, lockErr := readLockIfPresent(target)

	if result.ManifestExists && manifestErr != nil {
		result.Issues = append(result.Issues, manifestErr.Error())
		result.ManifestLockConsistent = false
	}
	if result.LockfileExists && lockErr != nil {
		result.Issues = append(result.Issues, lockErr.Error())
		result.ManifestLockConsistent = false
	}
	if result.ManifestExists && !result.LockfileExists {
		result.Issues = append(result.Issues, "manifest exists but lockfile is missing")
		result.ManifestLockConsistent = false
	}
	if !result.ManifestExists && result.LockfileExists {
		result.Issues = append(result.Issues, "lockfile exists but manifest is missing")
		result.ManifestLockConsistent = false
	}

	if result.ManifestExists && result.LockfileExists {
		lockNames := map[string]struct{}{}
		for _, pkg := range lockFile.Packages {
			lockNames[pkg.Name] = struct{}{}
		}
		for name := range manifestFile.Dependencies {
			if _, ok := lockNames[name]; !ok {
				result.Issues = append(result.Issues, fmt.Sprintf("manifest dependency %s is missing from the lockfile", name))
				result.ManifestLockConsistent = false
			}
		}
		if lockFile.WorkspacePath != "" && filepath.Clean(lockFile.WorkspacePath) != target {
			result.Issues = append(result.Issues, "lockfile workspacePath does not match the current workspace")
			result.ManifestLockConsistent = false
		}
	}

	filesToCheck := map[string]struct{}{}
	for _, pkg := range lockFile.Packages {
		for _, section := range pkg.ManagedSections {
			filesToCheck[section.File] = struct{}{}
		}
	}
	for _, candidate := range []string{"AGENTS.md", "SOUL.md", "TOOLS.md", "IDENTITY.md", "MEMORY.md"} {
		filesToCheck[candidate] = struct{}{}
	}

	for file := range filesToCheck {
		absolute := filepath.Join(target, file)
		data, err := os.ReadFile(absolute)
		if err != nil {
			continue
		}
		if !strings.Contains(string(data), "ocpm:begin") {
			continue
		}
		if err := managedsections.Validate(string(data)); err != nil {
			result.CorruptedManagedFiles = append(result.CorruptedManagedFiles, file)
			result.Issues = append(result.Issues, fmt.Sprintf("%s has malformed managed sections: %v", file, err))
			result.ManifestLockConsistent = false
		}
	}
	slices.Sort(result.CorruptedManagedFiles)
	slices.Sort(result.Issues)
	return result, nil
}

func (s *Service) resolveGraph(ctx context.Context, dependencies map[string]string) (map[string]registry.PackageVersion, error) {
	resolved := map[string]registry.PackageVersion{}
	constraints := map[string]string{}

	var visit func(name, constraint string) error
	visit = func(name, constraint string) error {
		if current, ok := constraints[name]; ok && current != "" && constraint != "" && current != constraint {
			return fmt.Errorf("dependency resolution conflict for %s: %s vs %s", name, current, constraint)
		}
		if _, ok := constraints[name]; !ok {
			constraints[name] = constraint
		}

		pkg, err := s.Registry.Resolve(ctx, name, constraint)
		if err != nil {
			return err
		}
		if existing, ok := resolved[name]; ok {
			if existing.Version != pkg.Version {
				return fmt.Errorf("dependency resolution conflict for %s: %s vs %s", name, existing.Version, pkg.Version)
			}
			return nil
		}

		resolved[name] = pkg
		for _, dependency := range pkg.Dependencies {
			if err := visit(dependency.Name, dependency.Constraint); err != nil {
				return err
			}
		}
		return nil
	}

	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		if err := visit(name, dependencies[name]); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func (s *Service) desiredPackages(resolved map[string]registry.PackageVersion, manifestFile manifest.File, currentLock lockfile.File, overridePackage string, overrideOptions map[string]string, prompter ui.Prompter) ([]materialize.DesiredPackage, map[string]map[string]string, error) {
	orderedNames := make([]string, 0, len(resolved))
	seen := map[string]struct{}{}

	topLevel := make([]string, 0, len(manifestFile.Dependencies))
	for name := range manifestFile.Dependencies {
		topLevel = append(topLevel, name)
	}
	slices.Sort(topLevel)
	for _, name := range topLevel {
		if _, ok := resolved[name]; ok {
			orderedNames = append(orderedNames, name)
			seen[name] = struct{}{}
		}
	}

	remaining := make([]string, 0, len(resolved))
	for name := range resolved {
		if _, ok := seen[name]; ok {
			continue
		}
		remaining = append(remaining, name)
	}
	slices.Sort(remaining)
	orderedNames = append(orderedNames, remaining...)

	var desired []materialize.DesiredPackage
	topLevelOptions := map[string]map[string]string{}

	for _, name := range orderedNames {
		pkg := resolved[name]
		previous := map[string]string{}
		if lockPkg, ok := currentLock.FindPackage(name); ok {
			previous = lockPkg.Options
		}

		manifestOptions := manifestFile.Options[name]
		provided := map[string]string{}
		if name == overridePackage {
			provided = overrideOptions
		}

		options, err := resolveOptions(pkg, manifestOptions, previous, provided, prompter)
		if err != nil {
			return nil, nil, err
		}

		if _, ok := manifestFile.Dependencies[name]; ok {
			topLevelOptions[name] = options
		}

		desired = append(desired, materialize.DesiredPackage{
			Package: pkg,
			Options: options,
		})
	}

	return desired, topLevelOptions, nil
}

func resolveOptions(pkg registry.PackageVersion, manifestValues, previousValues, provided map[string]string, prompter ui.Prompter) (map[string]string, error) {
	options := map[string]string{}
	known := map[string]registry.OptionSpec{}
	for _, spec := range pkg.InstallOptions {
		known[spec.Name] = spec
	}
	for key := range provided {
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("%s does not define an option named %q", pkg.Name, key)
		}
	}

	for _, spec := range pkg.InstallOptions {
		value := provided[spec.Name]
		if value == "" {
			value = manifestValues[spec.Name]
		}
		if value == "" {
			value = previousValues[spec.Name]
		}
		if value == "" {
			value = spec.Default
		}
		if value == "" && spec.Required {
			if prompter != nil && prompter.Interactive() {
				prompted, err := prompter.PromptOption(spec)
				if err != nil {
					return nil, err
				}
				value = prompted
			} else {
				return nil, fmt.Errorf("%s requires option %q; pass --option %s=<value>", pkg.Name, spec.Name, spec.Name)
			}
		}
		if value != "" {
			options[spec.Name] = value
		}
	}

	return options, nil
}

func (s *Service) persistWorkspaceState(workspacePath string, manifestFile manifest.File, lockFile lockfile.File, dryRun bool) ([]materialize.Operation, error) {
	manifestData, err := manifest.Marshal(manifestFile)
	if err != nil {
		return nil, err
	}
	lockData, err := lockfile.Marshal(lockFile)
	if err != nil {
		return nil, err
	}

	ops := []materialize.Operation{
		{Action: "write-file", Path: manifest.FileName},
		{Action: "write-file", Path: lockfile.FileName},
	}
	if dryRun {
		return ops, nil
	}

	if err := os.MkdirAll(filepath.Join(workspacePath, ".ocpm"), 0o755); err != nil {
		return nil, err
	}
	if err := writeIfChanged(filepath.Join(workspacePath, manifest.FileName), manifestData); err != nil {
		return nil, err
	}
	if err := writeIfChanged(filepath.Join(workspacePath, lockfile.FileName), lockData); err != nil {
		return nil, err
	}
	return ops, nil
}

func readOrDefaultManifest(path string, workspaceManifest bool) (manifest.File, error) {
	file, err := manifest.ReadFromDir(path)
	if err == nil {
		return file, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return manifest.Default(workspaceManifest), nil
	}
	return manifest.File{}, err
}

func readManifestIfPresent(path string) (manifest.File, error) {
	file, err := manifest.ReadFromDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return manifest.Default(true), nil
	}
	return file, err
}

func readLockIfPresent(path string) (lockfile.File, error) {
	file, err := lockfile.ReadFromDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return lockfile.New(path, time.Time{}), nil
	}
	return file, err
}

func readRequiredLock(path string) (lockfile.File, error) {
	file, err := lockfile.ReadFromDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lockfile.File{}, fmt.Errorf("%s is missing in %s", lockfile.FileName, path)
		}
		return lockfile.File{}, err
	}
	return file, nil
}

func writeIfChanged(path string, data []byte) error {
	current, err := os.ReadFile(path)
	if err == nil && string(current) == string(data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func dirEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func isAddSafeKind(kind registry.PackageKind) bool {
	return kind == registry.KindSkill || kind == registry.KindOverlay
}

func isCreateSafeKind(kind registry.PackageKind) bool {
	return kind == registry.KindAgent || kind == registry.KindWorkspaceTemplate
}

func defaultDirName(pkg string) string {
	pkg = strings.TrimPrefix(pkg, "@")
	pkg = strings.ReplaceAll(pkg, "/", "-")
	return pkg
}

func defaultInitManifest(target string, workspaceManifest bool, request InitRequest) (manifest.File, error) {
	file := manifest.Default(workspaceManifest)
	file.Private = request.Private
	file.Name = request.Name
	if file.Name == "" {
		file.Name = inferPackageName(target)
	}
	file.Version = request.Version
	if file.Version == "" {
		file.Version = "0.1.0"
	}
	file.Kind = request.Kind
	if file.Kind == "" {
		file.Kind = inferPackageKind(target, workspaceManifest)
	} else if !knownPackageKind(file.Kind) {
		return manifest.File{}, fmt.Errorf("invalid kind %q; use skill, overlay, workspace-template, or agent", file.Kind)
	}
	return file, nil
}

func inferPackageName(target string) string {
	name := filepath.Base(target)
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func inferPackageKind(target string, workspaceManifest bool) registry.PackageKind {
	if workspaceManifest {
		return registry.KindAgent
	}
	if pathIsDir(filepath.Join(target, "skills")) {
		return registry.KindSkill
	}
	if pathIsDir(filepath.Join(target, "templates")) {
		return registry.KindOverlay
	}
	return registry.KindSkill
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func knownPackageKind(kind registry.PackageKind) bool {
	switch kind {
	case registry.KindSkill, registry.KindOverlay, registry.KindWorkspaceTemplate, registry.KindAgent:
		return true
	default:
		return false
	}
}
