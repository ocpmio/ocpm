package config

import "os"

const DefaultAuthURL = "http://localhost:3000"

func ResolveAuthURL() string {
	if value := os.Getenv("OCPM_AUTH_URL"); value != "" {
		return value
	}
	return DefaultAuthURL
}
