package buildinfo

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type Info struct {
	Version string
	Commit  string
	Date    string
}

func Current() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}
