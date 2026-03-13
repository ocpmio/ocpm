package publish

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type matcher struct {
	patterns []string
}

func loadIgnoreFile(root string) (matcher, error) {
	file, err := os.Open(filepath.Join(root, ignoreFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return matcher{}, nil
		}
		return matcher{}, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, filepath.ToSlash(line))
	}
	if err := scanner.Err(); err != nil {
		return matcher{}, err
	}
	return matcher{patterns: patterns}, nil
}

func (m matcher) matches(path string, isDir bool) bool {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)

	for _, pattern := range m.patterns {
		if strings.HasSuffix(pattern, "/") {
			prefix := strings.TrimSuffix(pattern, "/")
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}
		if strings.Contains(pattern, "/") {
			if ok, _ := filepath.Match(pattern, path); ok {
				return true
			}
		} else {
			if ok, _ := filepath.Match(pattern, base); ok {
				return true
			}
			if isDir && pattern == path {
				return true
			}
		}
	}

	return false
}

func hardExcluded(path string) bool {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)

	switch {
	case path == ".git" || strings.HasPrefix(path, ".git/"):
		return true
	case path == ".ocpm" || strings.HasPrefix(path, ".ocpm/"):
		return true
	case path == "memory" || strings.HasPrefix(path, "memory/"):
		return true
	case path == "tmp" || strings.HasPrefix(path, "tmp/"):
		return true
	case path == "temp" || strings.HasPrefix(path, "temp/"):
		return true
	case path == "coverage" || strings.HasPrefix(path, "coverage/"):
		return true
	case path == "dist" || strings.HasPrefix(path, "dist/"):
		return true
	case path == "node_modules" || strings.HasPrefix(path, "node_modules/"):
		return true
	case path == ".env":
		return true
	case strings.HasPrefix(base, ".env."):
		return true
	case base == ".DS_Store":
		return true
	case strings.HasSuffix(base, ".log"):
		return true
	case strings.HasSuffix(base, ".tmp"):
		return true
	case base == "ocpm-lock.json":
		return true
	case base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock":
		return true
	case strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx"):
		return true
	default:
		return false
	}
}
