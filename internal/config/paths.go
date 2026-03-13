package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const DefaultAppName = "ocpm"

type Paths struct {
	ConfigDir string
	CacheDir  string
	StateDir  string
}

func Resolve(appName string) (Paths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	paths := Paths{
		ConfigDir: filepath.Join(configDir, appName),
		CacheDir:  filepath.Join(cacheDir, appName),
		StateDir:  filepath.Join(configDir, appName, "state"),
	}

	switch runtime.GOOS {
	case "darwin":
		paths.StateDir = filepath.Join(homeDir, "Library", "Application Support", appName, "state")
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			paths.StateDir = filepath.Join(localAppData, appName, "state")
		}
	default:
		paths.StateDir = filepath.Join(homeDir, ".local", "state", appName)
	}

	return paths, nil
}

func (p Paths) Ensure() error {
	for _, dir := range []string{p.ConfigDir, p.CacheDir, p.StateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return nil
}
