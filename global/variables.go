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
	Version:     "0.1.1",
	Developer:   "Madobi Nanami",
	BuildVer:    2,
	CommitHash:  "unknown",
	Description: "Remote server monitoring and command execution subsystem for Komari",
	BuildType:   "pre-release",
	BuildTime:   "unknown",
}

type ConfigPathStr struct {
	Global		  	string
	BotUserConfig 	string
	BotNodeConfig	string
}

var ConfigPath = ConfigPathStr{
	Global:		  	"config.json",
	BotUserConfig: 	"bot_user_config.json",
	BotNodeConfig:	"bot_node_config.json",
}