package materialize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/marian2js/ocpm/internal/lockfile"
	"github.com/marian2js/ocpm/internal/managedsections"
	"github.com/marian2js/ocpm/internal/registry"
)

type DesiredPackage struct {
	Package registry.PackageVersion
	Options map[string]string
}

type SyncRequest struct {
	WorkspacePath         string
	Current               lockfile.File
	Packages              []DesiredPackage
	DryRun                bool
	AllowDirectRootWrites bool
	Now                   time.Time
}

type Operation struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}

type SyncResult struct {
	Lock       lockfile.File `json:"lock"`
	Operations []Operation   `json:"operations"`
	Skipped    []Operation   `json:"skipped,omitempty"`
}

func Sync(request SyncRequest) (SyncResult, error) {
	if request.Now.IsZero() {
		request.Now = time.Now()
	}

	currentByName := map[string]lockfile.PackageLock{}
	currentByPath := map[string]string{}
	for _, pkg := range request.Current.Packages {
		currentByName[pkg.Name] = pkg
		for _, file := range pkg.InstalledFiles {
			currentByPath[file.Path] = pkg.Name
		}
	}

	desiredByName := map[string]DesiredPackage{}
	for _, pkg := range request.Packages {
		desiredByName[pkg.Package.Name] = pkg
	}

	result := SyncResult{
		Lock: lockfile.New(request.WorkspacePath, request.Now),
	}

	for _, current := range request.Current.Packages {
		desired, ok := desiredByName[current.Name]
		if ok && desired.Package.Version == current.Version {
			continue
		}
		ops, skipped, err := removePackage(request.WorkspacePath, current, request.DryRun)
		if err != nil {
			return SyncResult{}, err
		}
		result.Operations = append(result.Operations, ops...)
		result.Skipped = append(result.Skipped, skipped...)
	}

	for _, desired := range request.Packages {
		pkgLock, ops, skipped, err := applyPackage(request.WorkspacePath, desired, currentByPath, currentByName[desired.Package.Name], request.AllowDirectRootWrites, request.DryRun)
		if err != nil {
			return SyncResult{}, err
		}
		result.Lock.Packages = append(result.Lock.Packages, pkgLock)
		result.Operations = append(result.Operations, ops...)
		result.Skipped = append(result.Skipped, skipped...)
	}

	return result, nil
}

func removePackage(workspacePath string, pkg lockfile.PackageLock, dryRun bool) ([]Operation, []Operation, error) {
	var operations []Operation
	var skipped []Operation

	for _, section := range pkg.ManagedSections {
		absolute := filepath.Join(workspacePath, section.File)
		content, err := os.ReadFile(absolute)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}

		updated, removed, err := managedsections.Remove(string(content), section.Owner)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", section.File, err)
		}
		if !removed {
			continue
		}

		operations = append(operations, Operation{Action: "remove-section", Path: section.File, Detail: section.Owner})
		if !dryRun {
			if err := os.WriteFile(absolute, []byte(updated), 0o644); err != nil {
				return nil, nil, err
			}
		}
	}

	for _, file := range pkg.InstalledFiles {
		absolute := filepath.Join(workspacePath, file.Path)
		data, err := os.ReadFile(absolute)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}

		if file.Integrity != "" && file.Integrity != digest(data) {
			skipped = append(skipped, Operation{
				Action: "skip-remove",
				Path:   file.Path,
				Detail: "file was modified since installation",
			})
			continue
		}

		operations = append(operations, Operation{Action: "remove-file", Path: file.Path})
		if !dryRun {
			if err := os.Remove(absolute); err != nil && !os.IsNotExist(err) {
				return nil, nil, err
			}
			pruneEmptyParents(workspacePath, filepath.Dir(absolute))
		}
	}

	return operations, skipped, nil
}

func applyPackage(workspacePath string, desired DesiredPackage, currentByPath map[string]string, previous lockfile.PackageLock, allowDirectRootWrites bool, dryRun bool) (lockfile.PackageLock, []Operation, []Operation, error) {
	pkg := desired.Package
	result := lockfile.PackageLock{
		Name:        pkg.Name,
		Version:     pkg.Version,
		Kind:        pkg.Kind,
		Integrity:   pkg.Integrity,
		ResolvedURL: pkg.ResolvedURL,
		Options:     cloneMap(desired.Options),
	}
	var operations []Operation
	var skipped []Operation

	for _, dependency := range pkg.Dependencies {
		result.Dependencies = append(result.Dependencies, lockfile.Dependency{
			Name:       dependency.Name,
			Constraint: dependency.Constraint,
		})
	}

	payloadRoot := filepath.Join(".ocpm", "packages", packageDirName(pkg.Name), pkg.Version)
	for relPath, content := range pkg.Files {
		targetPath := filepath.ToSlash(filepath.Join(payloadRoot, relPath))
		fileOp, fileLock, err := writeManagedFile(workspacePath, targetPath, renderTemplate(content, desired.Options), pkg.Name, currentByPath, dryRun)
		if err != nil {
			return lockfile.PackageLock{}, nil, nil, err
		}
		if fileOp.Action != "" {
			operations = append(operations, fileOp)
		}
		result.InstalledFiles = append(result.InstalledFiles, fileLock)
	}

	for _, skill := range pkg.Skills {
		result.InstalledSkills = append(result.InstalledSkills, lockfile.InstalledSkill{
			Name: skill.Name,
			Path: filepath.ToSlash(filepath.Join("skills", skill.Name)),
		})
		for relPath, content := range skill.Files {
			targetPath := filepath.ToSlash(filepath.Join("skills", skill.Name, relPath))
			fileOp, fileLock, err := writeManagedFile(workspacePath, targetPath, renderTemplate(content, desired.Options), pkg.Name, currentByPath, dryRun)
			if err != nil {
				return lockfile.PackageLock{}, nil, nil, err
			}
			if fileOp.Action != "" {
				operations = append(operations, fileOp)
			}
			result.InstalledFiles = append(result.InstalledFiles, fileLock)
		}
	}

	for _, managed := range pkg.ManagedFiles {
		absolute := filepath.Join(workspacePath, managed.Path)
		content, err := os.ReadFile(absolute)
		switch {
		case err == nil:
		case os.IsNotExist(err) && (managed.CreateIfMissing || allowDirectRootWrites):
			content = []byte{}
		case os.IsNotExist(err):
			return lockfile.PackageLock{}, nil, nil, fmt.Errorf("%s is missing; %s cannot manage it safely", managed.Path, pkg.Name)
		default:
			return lockfile.PackageLock{}, nil, nil, err
		}

		updated, err := managedsections.Upsert(string(content), pkg.Name, renderTemplate(managed.Content, desired.Options))
		if err != nil {
			return lockfile.PackageLock{}, nil, nil, fmt.Errorf("%s: %w", managed.Path, err)
		}

		action := "update-section"
		if string(content) == "" {
			action = "create-section"
		}
		if string(content) == updated {
			action = ""
		}
		if action != "" {
			operations = append(operations, Operation{Action: action, Path: managed.Path, Detail: pkg.Name})
			if !dryRun {
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					return lockfile.PackageLock{}, nil, nil, err
				}
				if err := os.WriteFile(absolute, []byte(updated), 0o644); err != nil {
					return lockfile.PackageLock{}, nil, nil, err
				}
			}
		}

		result.ManagedSections = append(result.ManagedSections, lockfile.ManagedSection{
			File:  filepath.ToSlash(managed.Path),
			Owner: pkg.Name,
		})
	}

	if len(pkg.WorkspaceFiles) > 0 {
		if !allowDirectRootWrites {
			return lockfile.PackageLock{}, nil, nil, fmt.Errorf("%s is a %s package; use ocpm create instead of ocpm add", pkg.Name, pkg.Kind)
		}

		paths := make([]string, 0, len(pkg.WorkspaceFiles))
		for path := range pkg.WorkspaceFiles {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			fileOp, fileLock, err := writeManagedFile(workspacePath, path, renderTemplate(pkg.WorkspaceFiles[path], desired.Options), pkg.Name, currentByPath, dryRun)
			if err != nil {
				return lockfile.PackageLock{}, nil, nil, err
			}
			if fileOp.Action != "" {
				operations = append(operations, fileOp)
			}
			result.InstalledFiles = append(result.InstalledFiles, fileLock)
		}
	}

	if previous.Name != "" {
		for key, value := range previous.Options {
			if _, ok := result.Options[key]; !ok {
				result.Options[key] = value
			}
		}
	}

	return result, operations, skipped, nil
}

func writeManagedFile(workspacePath, relPath, content, owner string, currentByPath map[string]string, dryRun bool) (Operation, lockfile.InstalledFile, error) {
	absolute := filepath.Join(workspacePath, relPath)
	existing, err := os.ReadFile(absolute)
	switch {
	case err == nil:
		if string(existing) != content {
			currentOwner := currentByPath[filepath.ToSlash(relPath)]
			if currentOwner != "" && currentOwner != owner {
				return Operation{}, lockfile.InstalledFile{}, fmt.Errorf("refusing to overwrite %s because it is owned by %s", relPath, currentOwner)
			}
			if currentOwner == "" && !strings.HasPrefix(filepath.ToSlash(relPath), ".ocpm/packages/") {
				return Operation{}, lockfile.InstalledFile{}, fmt.Errorf("refusing to overwrite user-owned file %s", relPath)
			}
		}
	case os.IsNotExist(err):
	default:
		return Operation{}, lockfile.InstalledFile{}, err
	}

	action := "write-file"
	if err == nil && string(existing) == content {
		action = ""
	}

	if action != "" && !dryRun {
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			return Operation{}, lockfile.InstalledFile{}, err
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			return Operation{}, lockfile.InstalledFile{}, err
		}
	}

	return Operation{Action: action, Path: filepath.ToSlash(relPath)}, lockfile.InstalledFile{
		Path:      filepath.ToSlash(relPath),
		Integrity: digest([]byte(content)),
	}, nil
}

func renderTemplate(content string, options map[string]string) string {
	rendered := content
	for key, value := range options {
		rendered = strings.ReplaceAll(rendered, "{{option:"+key+"}}", value)
	}
	return rendered
}

func packageDirName(name string) string {
	replacer := strings.NewReplacer("@", "", "/", "__")
	return replacer.Replace(name)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func pruneEmptyParents(root, path string) {
	root = filepath.Clean(root)
	for current := filepath.Clean(path); strings.HasPrefix(current, root) && current != root; current = filepath.Dir(current) {
		entries, err := os.ReadDir(current)
		if err != nil || len(entries) > 0 {
			return
		}
		_ = os.Remove(current)
	}
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
