package buildinfo

// These values are replaced with -ldflags by release builds.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
}
