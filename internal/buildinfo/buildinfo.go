package buildinfo

import "strings"

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
	builtBy = "source"
)

type Info struct {
	Version string
	Commit  string
	Date    string
	BuiltBy string
}

func Current() Info {
	return Info{
		Version: valueOrDefault(version, "dev"),
		Commit:  valueOrDefault(commit, "unknown"),
		Date:    valueOrDefault(date, "unknown"),
		BuiltBy: valueOrDefault(builtBy, "source"),
	}
}

func (i Info) Summary() string {
	return "poe " + valueOrDefault(i.Version, "dev")
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
