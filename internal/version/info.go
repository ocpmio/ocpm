package version

type Info struct {
	Version string
	Commit  string
	Date    string
}

func New(version string, commit string, date string) Info {
	if version == "" {
		version = "dev"
	}

	if commit == "" {
		commit = "none"
	}

	if date == "" {
		date = "unknown"
	}

	return Info{
		Version: version,
		Commit:  commit,
		Date:    date,
	}
}

func (i Info) String() string {
	return i.Version + " (" + i.Commit + ", " + i.Date + ")"
}
