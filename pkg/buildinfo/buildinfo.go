package buildinfo

import "strings"

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = ""
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate,omitempty"`
}

func Current() Info {
	return Info{
		Version:   clean(Version, "dev"),
		Commit:    clean(Commit, "none"),
		BuildDate: strings.TrimSpace(BuildDate),
	}
}

func clean(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
