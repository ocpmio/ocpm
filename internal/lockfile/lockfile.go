package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/marian2js/ocpm/internal/registry"
)

const (
	FileName      = "ocpm-lock.json"
	SchemaVersion = 1
)

type File struct {
	Version       int           `json:"version"`
	WorkspacePath string        `json:"workspacePath"`
	GeneratedAt   string        `json:"generatedAt"`
	Packages      []PackageLock `json:"packages"`
}

type Dependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint,omitempty"`
	Version    string `json:"version,omitempty"`
}

type InstalledFile struct {
	Path      string `json:"path"`
	Integrity string `json:"integrity,omitempty"`
}

type ManagedSection struct {
	File  string `json:"file"`
	Owner string `json:"owner"`
}

type InstalledSkill struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type PackageLock struct {
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	Kind            registry.PackageKind `json:"kind"`
	Integrity       string               `json:"integrity"`
	ResolvedURL     string               `json:"resolvedURL"`
	Options         map[string]string    `json:"options,omitempty"`
	Dependencies    []Dependency         `json:"dependencies,omitempty"`
	InstalledFiles  []InstalledFile      `json:"installedFiles,omitempty"`
	ManagedSections []ManagedSection     `json:"managedSections,omitempty"`
	InstalledSkills []InstalledSkill     `json:"installedSkills,omitempty"`
}

func New(workspacePath string, now time.Time) File {
	return File{
		Version:       SchemaVersion,
		WorkspacePath: workspacePath,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		Packages:      []PackageLock{},
	}
}

func Read(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}

	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, err
	}

	file.normalize()
	return file, nil
}

func ReadFromDir(dir string) (File, error) {
	return Read(filepath.Join(dir, FileName))
}

func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, FileName))
	return err == nil
}

func Write(path string, file File) error {
	data, err := Marshal(file)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func WriteToDir(dir string, file File) error {
	return Write(filepath.Join(dir, FileName), file)
}

func Marshal(file File) ([]byte, error) {
	file.normalize()

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}

func (f *File) normalize() {
	if f.Version == 0 {
		f.Version = SchemaVersion
	}
	if f.Packages == nil {
		f.Packages = []PackageLock{}
	}

	for i := range f.Packages {
		pkg := &f.Packages[i]
		if pkg.Options == nil {
			pkg.Options = map[string]string{}
		}
		slices.SortFunc(pkg.Dependencies, func(a, b Dependency) int {
			if a.Name == b.Name {
				return cmpString(a.Version, b.Version)
			}
			return cmpString(a.Name, b.Name)
		})
		slices.SortFunc(pkg.InstalledFiles, func(a, b InstalledFile) int {
			return cmpString(a.Path, b.Path)
		})
		slices.SortFunc(pkg.ManagedSections, func(a, b ManagedSection) int {
			if a.File == b.File {
				return cmpString(a.Owner, b.Owner)
			}
			return cmpString(a.File, b.File)
		})
		slices.SortFunc(pkg.InstalledSkills, func(a, b InstalledSkill) int {
			return cmpString(a.Name, b.Name)
		})
	}

	slices.SortFunc(f.Packages, func(a, b PackageLock) int {
		return cmpString(a.Name, b.Name)
	})
}

func (f File) FindPackage(name string) (PackageLock, bool) {
	for _, pkg := range f.Packages {
		if pkg.Name == name {
			return pkg, true
		}
	}
	return PackageLock{}, false
}

func cmpString(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
