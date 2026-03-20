package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	ErrIntegrityMissing     = errors.New("package integrity is missing")
	ErrIntegrityUnsupported = errors.New("package integrity algorithm is not supported")
	ErrIntegrityMismatch    = errors.New("package integrity does not match resolved content")
)

func ComputePackageIntegrity(pkg PackageVersion) (string, error) {
	canonical := canonicalPackage(pkg)
	canonical.Integrity = ""

	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func VerifyPackage(pkg PackageVersion) error {
	integrity := strings.TrimSpace(pkg.Integrity)
	if integrity == "" {
		return ErrIntegrityMissing
	}

	algorithm, _, ok := strings.Cut(integrity, ":")
	if !ok || algorithm != "sha256" {
		return fmt.Errorf("%w: %s", ErrIntegrityUnsupported, integrity)
	}

	expected, err := ComputePackageIntegrity(pkg)
	if err != nil {
		return err
	}
	if expected != integrity {
		return fmt.Errorf("%w: expected %s got %s", ErrIntegrityMismatch, expected, integrity)
	}
	return nil
}

func canonicalPackage(pkg PackageVersion) PackageVersion {
	canonical := PackageVersion{
		Name:           pkg.Name,
		Version:        pkg.Version,
		Kind:           pkg.Kind,
		Integrity:      pkg.Integrity,
		ResolvedURL:    pkg.ResolvedURL,
		Files:          cloneStringMap(pkg.Files),
		WorkspaceFiles: cloneStringMap(pkg.WorkspaceFiles),
	}

	if len(pkg.Dependencies) > 0 {
		canonical.Dependencies = append([]Dependency(nil), pkg.Dependencies...)
		slices.SortFunc(canonical.Dependencies, func(a, b Dependency) int {
			switch {
			case a.Name < b.Name:
				return -1
			case a.Name > b.Name:
				return 1
			case a.Constraint < b.Constraint:
				return -1
			case a.Constraint > b.Constraint:
				return 1
			default:
				return 0
			}
		})
	}

	if len(pkg.ManagedFiles) > 0 {
		canonical.ManagedFiles = append([]ManagedFile(nil), pkg.ManagedFiles...)
		slices.SortFunc(canonical.ManagedFiles, func(a, b ManagedFile) int {
			switch {
			case a.Path < b.Path:
				return -1
			case a.Path > b.Path:
				return 1
			case a.Content < b.Content:
				return -1
			case a.Content > b.Content:
				return 1
			case !a.CreateIfMissing && b.CreateIfMissing:
				return -1
			case a.CreateIfMissing && !b.CreateIfMissing:
				return 1
			default:
				return 0
			}
		})
	}

	if len(pkg.InstallOptions) > 0 {
		canonical.InstallOptions = append([]OptionSpec(nil), pkg.InstallOptions...)
		slices.SortFunc(canonical.InstallOptions, func(a, b OptionSpec) int {
			switch {
			case a.Name < b.Name:
				return -1
			case a.Name > b.Name:
				return 1
			case a.Description < b.Description:
				return -1
			case a.Description > b.Description:
				return 1
			case a.Default < b.Default:
				return -1
			case a.Default > b.Default:
				return 1
			case !a.Required && b.Required:
				return -1
			case a.Required && !b.Required:
				return 1
			default:
				return 0
			}
		})
	}

	if len(pkg.Skills) > 0 {
		canonical.Skills = make([]Skill, len(pkg.Skills))
		for i, skill := range pkg.Skills {
			canonical.Skills[i] = Skill{
				Name:  skill.Name,
				Files: cloneStringMap(skill.Files),
			}
		}
		slices.SortFunc(canonical.Skills, func(a, b Skill) int {
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})
	}

	return canonical
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
