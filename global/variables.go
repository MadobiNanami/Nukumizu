package global

// SoftwareInfo holds build metadata.
type SoftwareInfoStr struct {
	Name        string
	Version     string
	Developer   string
	BuildVer    int16
	CommitHash  string
	Description string
	BuildType   string
	BuildTime   string
}

var SoftwareInfo = SoftwareInfoStr{
	Name:        "Nukumizu",
	Version:     "0.1.0",
	Developer:   "Madobi Nanami",
	BuildVer:    1,
	CommitHash:  "unknown",
	Description: "Remote server monitoring and command execution subsystem for Komari",
	BuildType:   "Debug",
	BuildTime:   "unknown",
}
