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
	Subject             string // Alert subject (see AlertParams)
	Source              string // Alert source (see AlertParams)
	Content             string // Alert content (see AlertParams)
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

// AlertTemplate is the body format of an alert submitted by an external
// application through the incoming webhook API.
const AlertTemplate = "{{ subject }}\n- Source: {{ source }}\n- Content:\n{{ content }}\n\n- Time: {{ time }}\nSent by Nukumizu Alert System"

// AlertParams holds the parameters of an alert submitted through the incoming
// webhook API.
type AlertParams struct {
	Subject string // Short one-line title of the alert
	Source  string // Name of the webhook endpoint the alert was submitted to
	Content string // Free-form alert body
	Time    string // Submission time
}

// RenderAlert renders the body of an alert for a channel. The alert source and
// content may be wrapped in Markdown — the source in inline code, the content
// in a fenced code block — when the target channel has markdown enabled
// (markdown); everything else, the timestamp included, stays plain text.
func RenderAlert(alert AlertParams, markdown bool) string {
	params := Params{
		Time:    alert.Time,
		Subject: alert.Subject,
		Source:  alert.Source,
		Content: alert.Content,
	}
	if markdown {
		params.Source = "`" + params.Source + "`"
		params.Content = "```\n" + params.Content + "\n```"
	}
	return Render(AlertTemplate, params, false)
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

// Render substitutes {{ paramName }} placeholders in a template string. The
// markdown argument is the target channel's markdown setting: when true the
// values that are meant to be read verbatim (UUIDs, messages, commands, command
// results) are wrapped in Markdown code spans and blocks, otherwise every value
// is inserted as plain text. Whether a channel renders Markdown comes from the
// configuration alone — the renderer never infers it from the channel name.
//
// Supported placeholders:
//   - {{ time }} — current server time
//   - {{ serverName }} — server name
//   - {{ serverUUID }} — server UUID
//   - {{ upStatus }} — "Online" or "Offline"
//   - {{ event }} — "Online" or "Offline"
//   - {{ message }} — event descriptive message
//   - {{ command }} — executed command
//   - {{ result }} — command execution result
//   - {{ subject }} — alert subject
//   - {{ source }} — alert source
//   - {{ content }} — alert content
//   - {{ list.onlineServers }} — multi-line online server list
//   - {{ list.offlineServers }} — multi-line offline server list
//   - {{ softwareVersion }} — software version
//   - {{ softwareBuildVer }} — software build version
//   - {{ softwareCommitHash }} — software commit hash
//   - {{ softwareBuildType }} — software build type (Debug/Release)
//   - {{ softwareBuildTime }} — software build time
//   - {{ softwareDeveloper }} — software developer
//   - {{ softwareDescription }} — software description
func Render(tmpl string, params Params, markdown bool) string {
	result := tmpl

	if markdown {
		// Channels that render Markdown get code blocks and inline code for the
		// values that are read verbatim.
		result = strings.ReplaceAll(result, "{{ time }}", "**"+params.Time+"**")
		result = strings.ReplaceAll(result, "{{ serverName }}", "**"+params.ServerName+"**")
		result = strings.ReplaceAll(result, "{{ serverUUID }}", "`"+params.ServerUUID+"`")
		result = strings.ReplaceAll(result, "{{ upStatus }}", "**"+params.UpStatus+"**")
		result = strings.ReplaceAll(result, "{{ event }}", "**"+params.Event+"**")
		result = strings.ReplaceAll(result, "{{ message }}", "`"+params.Message+"`")
		result = strings.ReplaceAll(result, "{{ command }}", "`"+params.Command+"`")
		result = strings.ReplaceAll(result, "{{ result }}", "```bash\n"+params.Result+"\n```")
		result = strings.ReplaceAll(result, "{{ list.onlineServers }}", params.OnlineServers)
		result = strings.ReplaceAll(result, "{{ list.offlineServers }}", params.OfflineServers)
		result = strings.ReplaceAll(result, "{{ softwareVersion }}", params.SoftwareVersion)
		result = strings.ReplaceAll(result, "{{ softwareBuildVer }}", fmt.Sprintf("%d", params.SoftwareBuildVer))
		result = strings.ReplaceAll(result, "{{ softwareCommitHash }}", params.SoftwareCommitHash)
		result = strings.ReplaceAll(result, "{{ softwareBuildType }}", params.SoftwareBuildType)
		result = strings.ReplaceAll(result, "{{ softwareBuildTime }}", params.SoftwareBuildTime)
		result = strings.ReplaceAll(result, "{{ softwareDeveloper }}", params.SoftwareDeveloper)
		result = strings.ReplaceAll(result, "{{ softwareDescription }}", params.SoftwareDescription)
	} else {
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
	}

	// Alert values carry their own formatting (see RenderAlert), so they are
	// substituted identically in both branches.
	result = strings.ReplaceAll(result, "{{ subject }}", params.Subject)
	result = strings.ReplaceAll(result, "{{ source }}", params.Source)
	result = strings.ReplaceAll(result, "{{ content }}", params.Content)

	return result
}

// FormatServerListEntry formats a single server entry for list display.
func FormatServerListEntry(name, uuid string) string {
	if name == "" {
		return fmt.Sprintf("- `%s`", uuid)
	}
	return fmt.Sprintf("- %s (`%s`)", name, uuid)
}

// JoinServerListEntries joins formatted server entries with newlines.
func JoinServerListEntries(entries []string) string {
	return strings.Join(entries, "\n")
}
