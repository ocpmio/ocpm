package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/marian2js/ocpm/internal/registry"
)

const FileName = "ocpm.json"

type File struct {
	Name          string                       `json:"name,omitempty"`
	Version       string                       `json:"version,omitempty"`
	Kind          registry.PackageKind         `json:"kind,omitempty"`
	Description   string                       `json:"description,omitempty"`
	License       string                       `json:"license,omitempty"`
	Repository    string                       `json:"repository,omitempty"`
	Homepage      string                       `json:"homepage,omitempty"`
	Keywords      []string                     `json:"keywords,omitempty"`
	Files         []string                     `json:"files,omitempty"`
	PublishConfig *PublishConfig               `json:"publishConfig,omitempty"`
	OCPM          *PackageMetadata             `json:"ocpm,omitempty"`
	Private       bool                         `json:"private,omitempty"`
	Workspace     bool                         `json:"workspace,omitempty"`
	Dependencies  map[string]string            `json:"dependencies,omitempty"`
	Options       map[string]map[string]string `json:"options,omitempty"`
}

type PublishConfig struct {
	Registry string `json:"registry,omitempty"`
	Access   string `json:"access,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

type PackageMetadata struct {
	Engines       map[string]string `json:"engines,omitempty"`
	Skills        []string          `json:"skills,omitempty"`
	Templates     []string          `json:"templates,omitempty"`
	Payload       []string          `json:"payload,omitempty"`
	Compatibility map[string]string `json:"compatibility,omitempty"`
}

func Default(workspace bool) File {
	return File{
		Private:      true,
		Workspace:    workspace,
		Dependencies: map[string]string{},
		Options:      map[string]map[string]string{},
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
	if f.Dependencies == nil {
		f.Dependencies = map[string]string{}
	}
	if f.Options == nil {
		f.Options = map[string]map[string]string{}
	}
	for pkg, values := range f.Options {
		if values == nil {
			f.Options[pkg] = map[string]string{}
		}
	}
	if f.OCPM != nil {
		if f.OCPM.Engines == nil {
			f.OCPM.Engines = map[string]string{}
		}
		if f.OCPM.Compatibility == nil {
			f.OCPM.Compatibility = map[string]string{}
		}
	}
}

func (f *File) RemoveDependency(name string) {
	if f.Dependencies != nil {
		delete(f.Dependencies, name)
	}
	if f.Options != nil {
		delete(f.Options, name)
	}
}

func (f *File) SetDependency(name, constraint string) {
	f.normalize()
	f.Dependencies[name] = constraint
}

func (f *File) SetOptions(name string, values map[string]string) {
	f.normalize()
	if len(values) == 0 {
		delete(f.Options, name)
		return
	}

	copyValues := make(map[string]string, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	f.Options[name] = copyValues
}

func MustRead(path string) File {
	file, err := Read(path)
	if err != nil {
		panic(err)
	}
	return file
}

func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
