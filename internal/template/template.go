package template

import (
	"fmt"
	"strings"
	"time"

	"nukumizu-backend/global"
	"nukumizu-backend/internal/node"
)

// Params holds all possible template parameters.
type Params struct {
	Time                string
	ServerName          string
	ServerUUID          string
	UpStatus            string
	Event               string
	Message             string
	Command             string
	Result              string
	OnlineServers       string // Pre-formatted multi-line list
	OfflineServers      string // Pre-formatted multi-line list
	SoftwareVersion     string
	SoftwareBuildVer    int16
	SoftwareCommitHash  string
	SoftwareBuildType   string
	SoftwareBuildTime   string
	SoftwareDeveloper   string
	SoftwareDescription string
}

// BuildBotInitializationMsgParams creates template parameters for the bot initialization message.
func BuildBotInitializationMsgParams() Params {
	return Params{
		Time:                time.Now().Format("2006-01-02T15:04:05.000000000-07:00"),
		SoftwareVersion:     global.SoftwareInfo.Version,
		SoftwareBuildVer:    global.SoftwareInfo.BuildVer,
		SoftwareCommitHash:  global.SoftwareInfo.CommitHash,
		SoftwareBuildType:   global.SoftwareInfo.BuildType,
		SoftwareBuildTime:   global.SoftwareInfo.BuildTime,
		SoftwareDeveloper:   global.SoftwareInfo.Developer,
		SoftwareDescription: global.SoftwareInfo.Description,
	}
}

// BuildParamsFromStatusChange creates template parameters from a status change event.
func BuildParamsFromStatusChange(change node.StatusChange) Params {
	return Params{
		Time:       time.Now().Format("2006-01-02T15:04:05.000000000-07:00"),
		ServerName: change.Name,
		ServerUUID: change.UUID,
		UpStatus:   change.Event,
		Event:      change.Event,
		Message:    change.Message,
	}
}

// BuildParamsFromServerList creates template parameters for the server list.
func BuildParamsFromServerList() Params {
	tracker := node.GetTracker()
	onlineServers := strings.Join(tracker.GetOnlineServers(), "\n")
	offlineServers := strings.Join(tracker.GetOfflineServers(), "\n")

	return Params{
		Time:           time.Now().Format("2006-01-02T15:04:05.000000000-07:00"),
		OnlineServers:  onlineServers,
		OfflineServers: offlineServers,
	}
}

// BuildParamsFromExecResult creates template parameters for a command execution result.
func BuildParamsFromExecResult(serverName, serverUUID, command, result string) Params {
	return Params{
		Time:       time.Now().Format("2006-01-02T15:04:05.000000000-07:00"),
		ServerName: serverName,
		ServerUUID: serverUUID,
		Command:    command,
		Result:     result,
	}
}

// Render substitutes {{ paramName }} placeholders in a template string.
// Supported placeholders:
//   - {{ time }} — current server time
//   - {{ serverName }} — server name
//   - {{ serverUUID }} — server UUID
//   - {{ upStatus }} — "Online" or "Offline"
//   - {{ event }} — "Online" or "Offline"
//   - {{ message }} — event descriptive message
//   - {{ command }} — executed command
//   - {{ result }} — command execution result
//   - {{ list.onlineServers }} — multi-line online server list
//   - {{ list.offlineServers }} — multi-line offline server list
//   - {{ softwareVersion }} — software version
//   - {{ softwareBuildVer }} — software build version
//   - {{ softwareCommitHash }} — software commit hash
//   - {{ softwareBuildType }} — software build type (Debug/Release)
//   - {{ softwareBuildTime }} — software build time
//   - {{ softwareDeveloper }} — software developer
//   - {{ softwareDescription }} — software description
func Render(tmpl string, params Params) string {
	result := tmpl

	result = strings.ReplaceAll(result, "{{ time }}", params.Time)
	result = strings.ReplaceAll(result, "{{ serverName }}", params.ServerName)
	result = strings.ReplaceAll(result, "{{ serverUUID }}", params.ServerUUID)
	result = strings.ReplaceAll(result, "{{ upStatus }}", params.UpStatus)
	result = strings.ReplaceAll(result, "{{ event }}", params.Event)
	result = strings.ReplaceAll(result, "{{ message }}", params.Message)
	result = strings.ReplaceAll(result, "{{ command }}", params.Command)
	result = strings.ReplaceAll(result, "{{ result }}", params.Result)
	result = strings.ReplaceAll(result, "{{ list.onlineServers }}", params.OnlineServers)
	result = strings.ReplaceAll(result, "{{ list.offlineServers }}", params.OfflineServers)
	result = strings.ReplaceAll(result, "{{ softwareVersion }}", params.SoftwareVersion)
	result = strings.ReplaceAll(result, "{{ softwareBuildVer }}", fmt.Sprintf("%d", params.SoftwareBuildVer))
	result = strings.ReplaceAll(result, "{{ softwareCommitHash }}", params.SoftwareCommitHash)
	result = strings.ReplaceAll(result, "{{ softwareBuildType }}", params.SoftwareBuildType)
	result = strings.ReplaceAll(result, "{{ softwareBuildTime }}", params.SoftwareBuildTime)
	result = strings.ReplaceAll(result, "{{ softwareDeveloper }}", params.SoftwareDeveloper)
	result = strings.ReplaceAll(result, "{{ softwareDescription }}", params.SoftwareDescription)

	return result
}

// FormatServerListEntry formats a single server entry for list display.
func FormatServerListEntry(name, uuid string) string {
	if name == "" {
		return fmt.Sprintf("- %s", uuid)
	}
	return fmt.Sprintf("- %s (%s)", name, uuid)
}

// JoinServerListEntries joins formatted server entries with newlines.
func JoinServerListEntries(entries []string) string {
	return strings.Join(entries, "\n")
}
