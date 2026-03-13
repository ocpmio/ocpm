package config

import "os"

const DefaultRegistryURL = "https://registry.openclaw.dev"

type RegistrySettings struct {
	URL   string
	Token string
}

func ResolveRegistrySettings(flagURL string, manifestURL string) RegistrySettings {
	url := flagURL
	if url == "" {
		url = os.Getenv("OCPM_REGISTRY_URL")
	}
	if url == "" {
		url = manifestURL
	}
	if url == "" {
		url = DefaultRegistryURL
	}

	return RegistrySettings{
		URL:   url,
		Token: os.Getenv("OCPM_TOKEN"),
	}
}
