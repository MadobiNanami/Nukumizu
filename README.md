# Nukumizu Backend

Remote server monitoring and command execution subsystem for [Komari](https://www.komari.wiki).

Nukumizu connects to a Komari Dashboard instance, keeps an in-memory view of every monitored server, and pushes alerts to several notification channels. It also runs interactive bots on QQ (via [NapCat](https://napneko.github.io/), OneBot 11) and Telegram so operators can control servers from chat.

## Features

- **Komari integration** — logs into the Komari Dashboard, refreshes the node list on startup and every 5 minutes, and polls each node's latest status through the Komari WebSocket every 5 seconds.
- **Status tracking** — a thread-safe node tracker keeps the latest report and static info per server and detects `Online` / `Offline` transitions.
- **Remote command execution** — dispatches commands through the Komari task API and polls the result (1s interval, up to 60s timeout).
- **Interactive bots** — QQ (NapCat / OneBot 11) and Telegram bots for `/list`, `/status`, `/info`, `/run`, `/shutdown`, `/reboot`, and more, protected by an admin / trusted-group permission model.
- **Notification channels** — server status changes are pushed to every enabled channel: QQ, Telegram, Email (SMTP), [ntfy](https://ntfy.sh), and Webhook.
- **Network proxy** — a global proxy URL can be enabled per controller (`networkUseProxy`) for HTTP, WebSocket, and even SMTP (HTTP CONNECT tunnel).
- **Customizable message templates** — every bot/notification message is rendered from a template in `config.json`.
- **Storage** — SQLite (pure-Go driver) for `user.db` and `log.db`; safe on network shares (WAL disabled).
- **Dashboard API** — token-authenticated REST API plus a live log-streaming WebSocket.

## How it works

1. On startup Nukumizu logs in to the Komari Dashboard. A failed login aborts the process.
2. It fetches the node list (name → UUID, static info) and stores it in memory, then opens a WebSocket to poll live status every 5 seconds. If the connection drops it reconnects with exponential backoff; after 5 failed attempts it notifies and keeps retrying.
3. The node tracker uses the first received snapshot as a baseline so a restart does not produce false "offline" alerts, then fires a status-change event for every real transition.
4. Each status-change event is rendered through the `SERVER_STATUS_CHANGED` template and sent to all enabled controllers. QQ / Telegram additionally receive a startup welcome and the initial server list.
5. Every 12 hours the Komari session is re-authenticated; the node list is refreshed every 5 minutes.

## Project structure

```
nukumizu-backend/
├── main.go                       # Entry point, startup sequence, graceful shutdown
├── router.go                     # HTTP route registration
├── config/
│   ├── config.go                 # Load config files, apply defaults
│   └── variables.go              # Config schema structs + globals
├── global/
│   └── variables.go              # Software build metadata (name/version/developer)
├── handler/
│   ├── user.go                   # /api/user/login, /api/user/register
│   ├── server.go                 # /api/server/list, getStatus, exec
│   └── health.go                 # /health
├── database/
│   └── user.go                   # user.db (SQLite) user store
├── utils/
│   ├── auth.go                   # Token management, Auth middleware, JSON responses
│   └── middleware.go             # Rate limit, CORS, XSS/security headers
├── postLog/                      # Logging subsystem
│   ├── postLog.go                # Leveled logger (stdout + broadcast)
│   ├── database.go               # log.db (SQLite, one table per run)
│   ├── logBroadcaster.go         # Fan-out to WebSocket clients
│   └── logSocketHandler.go       # /api/system/getLogs WebSocket handler
└── internal/
    ├── komari/
    │   ├── client.go             # Komari HTTP/JSON-RPC client (login, nodes, task exec/poll)
    │   └── ws.go                 # Komari status WebSocket (poll + reconnect)
    ├── node/
    │   └── tracker.go            # Thread-safe server state, status-change detection
    ├── netproxy/
    │   └── netproxy.go           # Unified network proxy for controllers
    ├── template/
    │   └── template.go           # Message template renderer ({{ variables }})
    └── controller/
        ├── controller.go         # Manager, Controller / BotController interfaces
        ├── trigger.go            # Command parsing, authorization, routing
        ├── processor.go          # Command handlers
        ├── utils.go
        └── pipes/
            ├── email.go          # Email notification pipe
            ├── ntfy.go           # ntfy notification pipe
            ├── webhook.go        # Webhook notification pipe
            ├── qq_napcat/
            │   ├── qq.go         # QQ (NapCat / OneBot 11) bot controller
            │   └── napcat.go     # NapCat WebSocket + HTTP API client
            └── telegram/
                ├── telegram.go   # Telegram bot controller (go-telegram/bot, long polling)
                └── send.go       # Message sending / splitting (Telegram Markdown)
```

## Requirements

- Go **1.25** or newer
- A running [Komari](https://www.komari.wiki) Dashboard instance reachable from this host
- For QQ: a [NapCat](https://napneko.github.io/) instance exposing an OneBot 11 WebSocket + HTTP endpoint
- For Telegram: a bot token from [@BotFather](https://t.me/BotFather)

Key dependencies: `github.com/go-telegram/bot`, `github.com/gorilla/websocket`, `gopkg.in/mail.v2`, `modernc.org/sqlite`.

## Configuration

There are two configuration files, both read from the working directory unless overridden:

| File | CLI flag | Default | Purpose |
|---|---|---|---|
| `config.json` | `-config` | `config.json` | Core settings: system, Komari, controllers, message templates |
| `bot_user_config.json` | `-bot-user-config` | `bot_user_config.json` | Per-bot admins / trusted groups and their notification preferences |

> Both files are in `.gitignore` because they contain credentials (Komari password, bot tokens, proxy auth). Start from the samples below and never commit real secrets.

### `config.json`

```json
{
    "system": {
        "debugMode": true,
        "listenAddr": "0.0.0.0",
        "listenPort": "8080",
        "networkProxy": "http://127.0.0.1:7890"
    },
    "debug": {
        "showNapcatMsg": false,
        "showNapcatAction": false,
        "showTelegramMsg": false,
        "showTriggerCmdEcho": true,
        "showKomariTaskEcho": false,
        "napcatIgnoreSelfMsg": false
    },
    "komari": {
        "dashboardURL": "https://status.example.com",
        "account": {
            "username": "admin",
            "password": "CHANGE_ME"
        }
    },
    "controllerMethod": {
        "qq(napcat)": {
            "enabled": false,
            "networkUseProxy": false,
            "napcatAddr": "127.0.0.1",
            "napcatPort": "3000",
            "napcatToken": "",
            "botQQID": 0,
            "listenMethod": "global"
        },
        "telegram": {
            "enabled": false,
            "networkUseProxy": false,
            "botToken": "",
            "listenMethod": "global"
        },
        "email": {
            "enabled": false,
            "networkUseProxy": false,
            "smtpHost": "",
            "smtpPort": 587,
            "username": "",
            "password": "",
            "from": "",
            "to": [],
            "useTLS": true
        },
        "ntfy": {
            "enabled": false,
            "networkUseProxy": false,
            "server": "https://ntfy.sh",
            "topic": "",
            "token": "",
            "priority": "default"
        },
        "webhook": {
            "enabled": false,
            "networkUseProxy": false,
            "url": "",
            "method": "POST",
            "headers": {},
            "template": ""
        }
    },
    "controllerMessage": {
        "BOT_STARTED": "Nukumizu Alert Bot Started\nTime: {{ time }}\n- Software Version: {{ softwareVersion }}\n- Build Version: {{ softwareBuildVer }}\n- Commit Hash: {{ softwareCommitHash }}\n- Build Type: {{ softwareBuildType }}\n- Build Time: {{ softwareBuildTime }}\n- Developer: {{ softwareDeveloper }}",
        "BOT_HELP": "Nukumizu Alert Bot Ver. {{ softwareVersion }}.{{ softwareBuildVer }}.{{ softwareCommitHash }}\nCommand Lists:\n- /help: Show this help message\n- /list: List all servers and show their status\n- /status <UUID>: Show specific server status\n- /info <UUID>: Show specific server info\n- /run <UUID> <command>: Execute command on specific server. If you type \"all\" in <UUID>, you will run the command on all servers.\n- /shutdown <UUID>: Shutdown specific server.\n- /reboot <UUID>: Reboot specific server.\n- /getip <UUID>: Get specific server IP address.",
        "TG_BOT_START": "Welcome to use Nukumizu Alert Bot!\nUse `/help` to get command list.",
        "SERVER_STATUS_CHANGED": "Server Status Changed Alert\n{{ serverName }} - {{ upStatus }}\n- Event: {{ event }}\n- Server Name: {{ serverName }}\n- Message: {{ message }}\n- Time: {{ time }}",
        "SERVER_LIST": "All server list:\n- Online:\n{{ list.onlineServers }}\n- Offline:\n{{ list.offlineServers }}",
        "SERVER_EXECUTE_RESULT": "Command execute result:\n- Server ID: {{ serverName }}\n- Command: {{ command }}\n-----**Result**-----\n\n{{ result }}\n----------\n\n- Time: {{ time }}"
    },
    "dataPath": "./data",
    "dbPath": "./db"
}
```

Field notes:

- `system.networkProxy` is a **system-wide** proxy URL. A controller only uses it when its own `networkUseProxy` is `true`. Applied to Telegram HTTP polling, NapCat HTTP/WebSocket, ntfy and webhook requests, and Email SMTP (tunneled via HTTP CONNECT).
- `controllerMethod.qq(napcat).listenMethod` / `telegram.listenMethod` — see [Bot recognition modes](#bot-recognition-modes).
- `debug` toggles verbose per-channel message/action logging; these only matter in debug builds / `debugMode`.
- `email.useTLS` is kept for configuration compatibility.
- `dataPath` / `dbPath` default to `./data` and `./db`; `user.db` and `log.db` are created under `dbPath`.
- Missing keys fall back to built-in defaults (host `0.0.0.0`, port `8080`, NapCat `127.0.0.1:3000`, ntfy server `https://ntfy.sh`, webhook method `POST`, etc.). Message templates have built-in fallbacks too.

### `bot_user_config.json`

Admins and trusted groups are defined **per bot channel** and map a member ID to that member's notification preferences:

```json
{
    "qq(napcat)": {
        "admins": {
            "123456789": {
                "event_status_notify": true,
                "event_bot_started": true,
                "event_reply": true
            }
        },
        "trustedGroups": {
            "987654321": {
                "event_status_notify": true,
                "event_bot_started": false,
                "event_reply": true
            }
        }
    },
    "telegram": {
        "admins": {
            "user_handle": {
                "event_status_notify": true,
                "event_bot_started": true,
                "event_reply": true
            }
        },
        "trustedGroups": {
            "-1001234567890": {
                "event_status_notify": true,
                "event_bot_started": false,
                "event_reply": true
            }
        }
    }
}
```

- **QQ**: member IDs are QQ numbers; group IDs are the numeric group number.
- **Telegram**: member IDs may be `@username` (resolved to the numeric user ID once that user has messaged the bot) or the numeric user ID; group IDs are the numeric chat ID (supergroups are negative).
- Per-member options:
  - `event_status_notify` — receive server `Online`/`Offline` push notifications.
  - `event_bot_started` — receive the automatic startup message (welcome + initial server list).
  - `event_reply` — reserved for opting out of replies to that member's own commands.
- Admins of a channel also appear in every trusted-group/private-chat context where the bot sends notifications.

### Message templates

`controllerMessage` templates are rendered before sending. Available variables (rendered through the Telegram pipe are additionally wrapped in Telegram legacy Markdown):

| Variable | Meaning |
|---|---|
| `{{ time }}` | Current server time |
| `{{ serverName }}` | Targeted server name |
| `{{ serverUUID }}` | Targeted server UUID |
| `{{ upStatus }}` | `Online` / `Offline` |
| `{{ event }}` | Status change event (`Online` / `Offline`) |
| `{{ message }}` | Message accompanying a status event |
| `{{ command }}` | The command that was executed |
| `{{ result }}` | Command execution result |
| `{{ list.onlineServers }}` | Formatted list of online servers (`- Name (uuid)`) |
| `{{ list.offlineServers }}` | Formatted list of offline servers |
| `{{ softwareVersion }}`, `{{ softwareBuildVer }}`, `{{ softwareCommitHash }}`, `{{ softwareBuildType }}`, `{{ softwareBuildTime }}`, `{{ softwareDeveloper }}`, `{{ softwareDescription }}` | Build metadata (commit hash and build time are injected at compile time) |

## API

All responses follow the envelope `{"success": true|false, "message": "...", ...data}`. `message` is empty on success unless noted.

### Authentication

Requests are authenticated with HTTP headers:

| Header | Meaning |
|---|---|
| `X-Token` | Token returned by login/register. Held in memory only (lost on restart). |
| `X-Timestamp` | Unix timestamp (seconds); rejected if more than ±30 minutes from server time. **Skipped entirely when `system.debugMode` is `true`.** |

Tokens idle for more than 1 hour are expired (cleaned every 10 minutes); any authenticated call refreshes the timer. Permission levels: `None`, `bot`, `admin`. Endpoints requiring `bot` accept both `bot` and `admin` tokens. Currently registration/login always issue `admin`-level tokens.

### Endpoints

| Endpoint | Method | Permission | Description |
|---|---|---|---|
| `/api/user/login` | POST | None | Log in. Body `{username, password}`. Returns `{token, userID, username, level, registerDate}`. |
| `/api/user/register` | POST | None | Register the first user. Body `{username, password}`. Only allowed while no user exists; otherwise `403`. Returns `{token, userID, username, level}`. |
| `/api/server/list` | GET | bot / admin | List all monitored servers. |
| `/api/server/getStatus` | GET | bot / admin | Recent live status for a server. Query `?uuid=<uuid>`. Returns `{uuid, report}` or `404`. |
| `/api/server/exec` | POST | bot / admin | Execute a command. Body `{uuid: [<uuid>...], command}`. Dispatches a Komari task and polls until completion (or timeout). Returns `{taskID, results}`. |
| `/health` | GET | None | Health check. Returns `{status, database}`. |
| `/api/system/getLogs` | WebSocket | None | Streams logs. Sends the last 100 buffered entries, then live `{level, content, timestamp}` events. |

Middleware applied to the whole server:

- **Rate limit** — token bucket, 100 requests/minute per client IP.
- **CORS** — `Access-Control-Allow-Origin: *`, allows `Content-Type`, `X-Token`, `X-Timestamp`, `Authorization`.
- **Security headers** — `X-XSS-Protection`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, a restrictive CSP.

## Bots

QQ (NapCat) and Telegram bots share one command engine and authorization pipeline, implemented in `internal/controller/`. NapCat speaks OneBot 11 (WebSocket event stream + HTTP actions); Telegram uses `go-telegram/bot` long polling.

### Permission model

- **Trusted group**: any command issued *inside a group* is only answered if that group is listed in `trustedGroups`. Messages in other groups are ignored.
- **Admin commands**: `/shutdown`, `/reboot`, and `/run` additionally require the *sender* to be listed in `admins`.
- **Private chat**: non-admin commands (`/help`, `/list`, `/status`, `/info`, `/getip`) are answered for any private sender; admin commands still require admin.

### Bot recognition modes

- `global` — the bot watches all messages in trusted groups and reacts to recognized commands without being mentioned. Unknown `/`-commands are silently ignored.
- `at` — the bot only reacts when it is mentioned (QQ `@`, Telegram `@botname`). In this mode an unknown command produces an `Unknown command: /…` reply.

### Commands

| Command | Permission | Description |
|---|---|---|
| `/help` | All | Show the help message (`BOT_HELP` template). |
| `/list` | All | List all servers with online/offline state (`SERVER_LIST` template). |
| `/status <uuid>` | All | Live report for a server (CPU, RAM, disk, network, uptime, processes). |
| `/info <uuid>` | All | Static info for a server (OS, kernel, CPU, RAM, swap, disk, billing, tags). |
| `/getip <uuid>` | All | IPv4 / IPv6 address of a server. |
| `/shutdown <uuid>` | Admin | Shut the server down via the Komari task API. |
| `/reboot <uuid>` | Admin | Reboot the server via the Komari task API. |
| `/run <uuid\|all> <command>` | Admin | Execute a command on one server or on all servers (`all`), then report the result (`SERVER_EXECUTE_RESULT` template). |
| `/start` | All | Telegram only — sends the `TG_BOT_START` welcome message. |

## Notification channels

QQ and Telegram are *interactive* channels. Email, ntfy, and webhook are **status-only** channels — they receive server status-change alerts but cannot run commands. On startup, the welcome message and initial server list are delivered only to the bot channels (QQ / Telegram), honoring each member's `event_bot_started` preference.

## Building

Requires Go 1.25+. Repo includes helper scripts that bake the current git commit and build time into the binary via `-ldflags`:

```bash
# Windows (cross-compile to Linux amd64)
build-linux-x86_64.bat

# Windows amd64
build-win-x86_64.bat
```

Equivalent manual builds:

```bash
# Linux / macOS
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags "-X main.CommitHash=$(git rev-parse --short HEAD) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o nukumizu-linux-amd64 .

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -ldflags "-X main.CommitHash=<commit> -X main.BuildTime=<utc-time>" \
  -o nukumizu-windows-amd64.exe .
```

`main.CommitHash` and `main.BuildTime` are surfaced in logs and in the `BOT_STARTED` message.

## Running

Both `config.json` and `bot_user_config.json` must exist in the working directory (or be passed explicitly):

```bash
# Development (uses go run, so config files must be in the CWD)
run.bat

# Or build first, then run a binary
./nukumizu-linux-amd64 -config config.json -bot-user-config bot_user_config.json
```

On startup the program logs in to Komari, loads node state, connects the status WebSocket, then starts each enabled controller and the HTTP server on `listenAddr:listenPort`. Press `Ctrl+C` for a graceful shutdown.

## License

See [LICENSE](LICENSE).
