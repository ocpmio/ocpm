package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/marian2js/ocpm/internal/openclaw"
)

var rootBootstrapFiles = []string{
	"SOUL.md",
	"IDENTITY.md",
	"TOOLS.md",
	"MEMORY.md",
}

type Detection struct {
	Path               string   `json:"path"`
	LooksLikeWorkspace bool     `json:"looksLikeWorkspace"`
	Indicators         []string `json:"indicators,omitempty"`
}

type Resolution struct {
	Path      string    `json:"path"`
	Source    string    `json:"source"`
	Detection Detection `json:"detection"`
}

type Resolver struct {
	openclaw openclaw.Client
}

func NewResolver(client openclaw.Client) *Resolver {
	return &Resolver{openclaw: client}
}

func Detect(path string) (Detection, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Detection{}, err
	}
	if !info.IsDir() {
		return Detection{}, fmt.Errorf("%s is not a directory", path)
	}

	indicators := []string{}

	if fileExists(filepath.Join(path, "AGENTS.md")) {
		indicators = append(indicators, "AGENTS.md")
	}
	if dirExists(filepath.Join(path, ".ocpm")) {
		indicators = append(indicators, ".ocpm/")
	}
	if dirExists(filepath.Join(path, "skills")) {
		for _, candidate := range rootBootstrapFiles {
			if fileExists(filepath.Join(path, candidate)) {
				indicators = append(indicators, "skills/+"+candidate)
				break
			}
		}
	}

	return Detection{
		Path:               filepath.Clean(path),
		LooksLikeWorkspace: len(indicators) > 0,
		Indicators:         indicators,
	}, nil
}

func (r *Resolver) Resolve(ctx context.Context, cwd, explicit string) (Resolution, error) {
	if explicit != "" {
		detection, err := Detect(explicit)
		if err != nil {
			return Resolution{}, err
		}
		if !detection.LooksLikeWorkspace {
			return Resolution{}, fmt.Errorf("%s is not an OpenClaw workspace; pass a workspace path that contains AGENTS.md, .ocpm/, or skills/ with bootstrap files", explicit)
		}
		return Resolution{Path: detection.Path, Source: "flag", Detection: detection}, nil
	}

	detection, err := Detect(cwd)
	if err == nil && detection.LooksLikeWorkspace {
		return Resolution{Path: detection.Path, Source: "cwd", Detection: detection}, nil
	}

	if r.openclaw != nil && r.openclaw.IsInstalled(ctx) {
		defaultPath, err := r.openclaw.DefaultWorkspace(ctx)
		if err == nil && defaultPath != "" {
			detection, detectErr := Detect(defaultPath)
			if detectErr == nil && detection.LooksLikeWorkspace {
				return Resolution{Path: detection.Path, Source: "openclaw-default", Detection: detection}, nil
			}
		}
	}

	return Resolution{}, fmt.Errorf("no target workspace found; use --workspace <path>, run from an OpenClaw workspace, or configure an OpenClaw default workspace")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
