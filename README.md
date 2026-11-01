# Nukumizu Backend

English | [简体中文](README.zh-CN.md)

Remote server monitoring and command execution subsystem for [Komari](https://www.komari.wiki).

Nukumizu connects to a Komari Dashboard instance, keeps an in-memory view of every monitored server, and pushes alerts to several notification channels. It also runs interactive bots on QQ (via [NapCat](https://napneko.github.io/) or any OneBot 11 implementation) and Telegram so operators can control servers from chat. A Vue console embedded in the binary provides the same overview and settings from a browser.

## Features

- **Komari integration** — logs into the Komari Dashboard, refreshing the node list on startup and every 5 minutes, and pulls each node's latest status over the Komari WebSocket every 5 seconds.
- **Resilient upstream access** — login and node fetching are retried up to 6 attempts with linear backoff (0.5s × attempt) on transport errors, `429` and `5xx`. Credential failures, `4xx` responses and JSON-RPC errors fail immediately instead of hammering the dashboard.
- **Status tracking** — a thread-safe node tracker keeps the latest report and static info per server and detects `Online` / `Offline` transitions. The first snapshot after startup is treated as a baseline, so a restart does not produce a wave of false "offline" alerts.
- **Per-node notification control** — `bot_node_config.json` can silence status pushes for an individual node, and can hold an `Offline` notification for a configurable window so a quick reconnect produces no alert at all (flap inhibition).
- **Remote command execution** — dispatches commands through the Komari task API and polls the result (1s interval, up to 60s timeout).
- **Interactive bots** — QQ (OneBot 11, e.g. NapCat) and Telegram bots for `/list`, `/status`, `/info`, `/run`, `/shutdown`, `/reboot`, and more, protected by an admin / trusted-group permission model.
- **Notification channels** — status changes are pushed to every enabled channel: QQ, Telegram, Email (SMTP), [ntfy](https://ntfy.sh), and Webhook.
- **Incoming webhook API** — external applications can push their own alerts in via `POST /api/webhook/post/<name>`, and Nukumizu relays them to the channels that endpoint lists. Each endpoint carries its own token and target channels, and the API is served on a **separate listener** so it can be exposed without exposing the admin API.
- **Network proxy** — a global proxy URL can be enabled per controller (`networkUseProxy`) for HTTP, WebSocket, and even SMTP (HTTP CONNECT tunnel).
- **Customizable message templates** — every bot/notification message is rendered from a template in `config.json`, with Markdown formatting switched on per channel.
- **Hot reload** — saving settings applies almost everything immediately; the few keys that are only read at startup are reported back as `restartRequired`.
- **Storage** — SQLite (pure-Go driver) for `user.db` and `log.db`; the rollback journal is used instead of WAL so the files stay safe on network shares.
- **Dashboard API** — token-authenticated REST API plus an admin-only live log-streaming WebSocket.
- **Web console** — a Vue 3 admin UI for browsing nodes and their per-node options, editing settings, managing bot trust and webhook endpoints, and tailing logs. The built bundle is embedded in the binary, so a single executable serves both the API and the console.
- **Tests** — the Go packages ship with unit tests (`go test ./...`), including the retry, flap-inhibition and settings-merge paths.

## How it works

1. On startup Nukumizu loads `config.json`, `bot_user_config.json` and (optionally) `bot_node_config.json`, then logs in to the Komari Dashboard. Login retries transport errors / `429` / `5xx` up to 6 times; a login that still fails aborts the process.
2. It fetches the node list (name → UUID, static info) and stores it in memory, then opens a WebSocket to `/api/rpc2` and requests the latest status every 5 seconds. If the connection drops it reconnects with exponential backoff (1s, 2s, 4s … capped at 30s); after 5 failed attempts it logs an error and keeps retrying every 30 seconds.
3. The node tracker waits for the initial snapshot covering every known node before enabling notifications (with a 2-minute safety net), and always treats the very first snapshot as the startup baseline — no alerts are generated for the state found at boot.
4. Each real transition is rendered through the `SERVER_STATUS_CHANGED` template and sent to all enabled controllers. Before that, the per-node gates from `bot_node_config.json` are applied: `enableStatusNotify` can drop the change entirely, and `flap_tolerant` holds an `Offline` for N seconds — if the node returns in time, both the `Offline` and the `Online` are suppressed.
5. QQ / Telegram additionally receive a startup welcome message and the initial server list once the first refresh completes, honoring each member's `event_bot_started` preference.
6. Every 12 hours the Komari session is re-authenticated; the node list is refreshed every 5 minutes.

## Project structure

```
nukumizu-backend/
├── main.go                       # Entry point, startup sequence, background tasks, graceful shutdown
├── router.go                     # HTTP route registration (main + webhook API)
├── config/
│   ├── config.go                 # Load config files, apply defaults, per-node helpers
│   ├── variables.go              # Config schema structs + atomic config singletons
│   ├── settings.go               # Settings file read/merge/write, startup-only key detection
│   ├── webhook.go                # Incoming webhook endpoint helpers (add/modify/delete)
│   ├── reload.go                 # Reload hooks (logger, controller rebuild)
│   └── *_test.go                 # Settings merge, concurrency and flap tests
├── global/
│   └── variables.go              # Build metadata (name/version/developer) + config paths
├── handler/
│   ├── user.go                   # /api/user/login, /api/user/register
│   ├── server.go                 # /api/server/list, getInfo, getStatus, exec
│   ├── settings.go               # /api/settings/get, set
│   ├── webhook.go                # /api/webhook/post/{name} (incoming webhook API)
│   ├── webhook_endpoints.go      # /api/webhook/add, modify, delete, list
│   └── health.go                 # /health
├── database/
│   └── user.go                   # user.db (SQLite) user store
├── utils/
│   ├── auth.go                   # Token management, Auth middleware, JSON responses
│   └── middleware.go             # Rate limit, CORS, security headers
├── postLog/                      # Logging subsystem
│   ├── postLog.go                # Leveled logger (stdout + broadcast)
│   ├── database.go               # log.db (SQLite, one table per run)
│   ├── logBroadcaster.go         # Fan-out to WebSocket clients
│   └── logSocketHandler.go       # /api/system/getLogs WebSocket handler (admin only)
├── web/
│   ├── embed.go                  # Embeds the built console (web/dist) in the binary
│   ├── dist/                     # Vite build output — generated, gitignored
│   └── handler.go                # Static file serving + SPA fallback
├── internal/
│   ├── komari/
│   │   ├── client.go             # Komari HTTP/JSON-RPC client (login, nodes, task exec/poll)
│   │   └── ws.go                 # Komari status WebSocket (poll + reconnect)
│   ├── node/
│   │   └── tracker.go            # Thread-safe server state, baseline gating, transitions
│   ├── netproxy/
│   │   └── netproxy.go           # Unified network proxy for controllers
│   ├── template/
│   │   └── template.go           # Message template renderer ({{ variables }})
│   └── controller/
│       ├── controller.go         # Manager, interfaces, alert routing, flap inhibition
│       ├── trigger.go            # Command parsing, authorization, routing
│       ├── processor.go          # Command handlers
│       ├── utils.go
│       └── pipes/
│           ├── email.go          # Email notification pipe
│           ├── ntfy.go           # ntfy notification pipe
│           ├── webhook.go        # Outgoing webhook notification pipe
│           ├── qq_onebot/
│           │   ├── qq.go         # QQ controller (OneBot 11, e.g. NapCat)
│           │   └── onebot.go     # OneBot 11 WebSocket client (events + actions)
│           └── telegram/
│               ├── telegram.go   # Telegram bot controller (go-telegram/bot, long polling)
│               └── send.go       # Message sending / splitting (Telegram Markdown)
└── frontend/                     # Vue 3 admin console (Vite)
    ├── index.html
    ├── vite.config.js            # Dev server; proxies /api to the backend
    └── src/
        ├── main.js               # Bootstrap: theme + runtime flags
        ├── App.vue               # Root component + toast host
        ├── api/index.js          # Wrappers around the REST endpoints
        ├── router/index.js       # Routes and the login guard
        ├── styles/               # theme.css + ui.css
        ├── utils/                # http/auth/theme/toast/format/runtime helpers
        ├── components/           # Modal, Toggle, editors, ConfigSection, top bar, side bar
        └── views/                # Login, Layout, Overview, Trusted, Settings, WebHooks, Logs
```

## Requirements

- Go **1.27.3** or newer (see `go.mod`)
- [Node.js](https://nodejs.org) **22** or newer — only needed to build the web console; a prebuilt binary does not require it
- A running [Komari](https://www.komari.wiki) Dashboard instance reachable from this host
- For QQ: an OneBot 11 implementation exposing a forward WebSocket endpoint, e.g. [NapCat](https://napneko.github.io/) (`/api` and `/event`)
- For Telegram: a bot token from [@BotFather](https://t.me/BotFather)

Key dependencies: `github.com/go-telegram/bot`, `github.com/gorilla/websocket`, `gopkg.in/mail.v2`, `modernc.org/sqlite`.

## Configuration

Three JSON files are read from the working directory. **There are no command-line flags**: the paths are fixed (`config.json`, `bot_user_config.json`, `bot_node_config.json`), so start the program from the directory that holds them.

| File | Required | Purpose |
|---|---|---|
| `config.json` | Yes | Core settings: system, debug switches, Komari, webhook listener, controllers, message templates, storage paths |
| `bot_user_config.json` | Yes | Per-bot admins / trusted groups and their notification preferences |
| `bot_node_config.json` | No | Per-node options (notification switch, flap window). Auto-created on the first node fetch |

> All three files are in `.gitignore` because they contain credentials (Komari password, bot tokens, proxy auth). Start from the samples below and never commit real secrets. `bot_node_config.json` is safe to keep, but it is ignored along with the others.

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
        "showOneBotMsg": false,
        "showOneBotAction": false,
        "showTelegramMsg": false,
        "showTriggerCmdEcho": true,
        "showKomariTaskEcho": false,
        "oneBotIgnoreSelfMsg": false,
        "showFlapTolerantEcho": false
    },
    "komari": {
        "dashboardURL": "https://status.example.com",
        "account": {
            "username": "admin",
            "password": "CHANGE_ME"
        }
    },
    "webhook": {
        "enabled": false,
        "listenAddr": "0.0.0.0",
        "listenPort": "8081",
        "endpoints": {
            "example": {
                "enabled": true,
                "token": "CHANGE_ME",
                "notifyPipes": ["qq_onebot", "telegram", "email", "ntfy"]
            }
        }
    },
    "controllerMethod": {
        "qq_onebot": {
            "enabled": false,
            "markdown": false,
            "networkUseProxy": false,
            "oneBotAddr": "127.0.0.1",
            "oneBotPort": "3000",
            "oneBotToken": "",
            "botQQID": 0,
            "listenMethod": "global"
        },
        "telegram": {
            "enabled": false,
            "markdown": true,
            "networkUseProxy": false,
            "botToken": "",
            "listenMethod": "global"
        },
        "email": {
            "enabled": false,
            "markdown": false,
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
            "markdown": false,
            "networkUseProxy": false,
            "server": "https://ntfy.sh",
            "topic": "",
            "token": "",
            "priority": "default"
        },
        "webhook": {
            "enabled": false,
            "markdown": false,
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

- `controllerMethod` is keyed by channel name: `qq_onebot`, `telegram`, `email`, `ntfy`, `webhook`. The same names are used in `notifyPipes` and in `bot_user_config.json`. The QQ channel speaks OneBot 11, so `oneBotAddr` / `oneBotPort` / `oneBotToken` point at the OneBot implementation (NapCat by default on `127.0.0.1:3000`).
- `system.networkProxy` is a **system-wide** proxy URL. A controller only uses it when its own `networkUseProxy` is `true`. Applied to Telegram HTTP polling, the QQ OneBot WebSocket, ntfy and webhook requests, and Email SMTP (tunneled via HTTP CONNECT).
- `webhook` configures the **incoming** webhook API (see [Incoming webhook API](#incoming-webhook-api)); `controllerMethod.webhook` configures the outgoing webhook notification channel. They are independent.
- `markdown` is a per-channel switch on all five channels. With it `false` (the default) every rendered value is inserted as plain text; with it `true` the values meant to be read verbatim (UUIDs, event messages, commands, command results, alert source and alert content) are wrapped in Markdown code spans / fenced blocks. Nothing is inferred from the channel name, so a channel only ever gets the formatting you asked for — turn it off for a channel whose platform does not render Markdown. On Telegram it also picks the `parse_mode`: with `markdown` off, messages are sent without one, so text containing `*` or `_` is delivered as-is rather than rejected by the API as malformed Markdown.
- `controllerMethod.qq_onebot.listenMethod` / `telegram.listenMethod` — see [Bot recognition modes](#bot-recognition-modes).
- `debug` toggles verbose per-module logging; these only matter when `system.debugMode` is `true`:
  - `showOneBotMsg` / `showOneBotAction` — incoming OneBot messages / outgoing OneBot API calls.
  - `showTelegramMsg` — incoming Telegram updates.
  - `showTriggerCmdEcho` — the parsed command handed to the trigger.
  - `showKomariTaskEcho` — Komari task creation and polling.
  - `oneBotIgnoreSelfMsg` — ignore the bot's own messages echoed back by OneBot.
  - `showFlapTolerantEcho` — the flap-inhibition decisions (held / released / reported).
- `email.useTLS` is kept for configuration compatibility.
- `dbPath` defaults to `./db`; `user.db` and `log.db` are created under it. `dataPath` defaults to `./data` and is reserved for future file storage — nothing is written there yet.
- Missing keys fall back to built-in defaults (host `0.0.0.0`, port `8080`, webhook API `0.0.0.0:8081`, no webhook endpoints, OneBot `127.0.0.1:3000`, ntfy server `https://ntfy.sh`, webhook method `POST`, etc.). Message templates have built-in fallbacks too. `markdown` defaults to `false`, so add it explicitly for Telegram (see the sample above) to keep its formatting.

### `bot_user_config.json`

Admins and trusted groups are defined **per bot channel** and map a member ID to that member's notification preferences:

```json
{
    "qq_onebot": {
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
  - `event_bot_started` — receive the automatic startup messages (welcome + initial server list).
  - `event_reply` — reserved for opting out of replies to that member's own commands.
- Admins of a channel also appear in every trusted-group/private-chat context where the bot sends notifications.

### `bot_node_config.json`

Per-node options, keyed by the node UUID that Komari reports:

```json
{
    "0f2a9c1e-1234-4a5b-8c9d-000000000001": {
        "enableStatusNotify": false,
        "flap_tolerant": 30
    },
    "7d3b8f20-5678-4c9d-9e0f-000000000002": {}
}
```

| Field | Default | Meaning |
|---|---|---|
| `enableStatusNotify` | `true` | When `false`, **no** status-change notification is sent for this node — through chat bots and notification pipes alike. Omit it (or the whole entry) to keep notifications on. |
| `flap_tolerant` | `0` | Seconds an `Offline` notification is held (float allowed, capped at 86400). If the node comes back within the window, both the `Offline` and the `Online` are suppressed; if it is still down when the window elapses, the `Offline` is sent then. `0` or an absent field notifies immediately. |

The file is auto-generated: every node fetch (`FetchNodes` — startup, reconnect and the 5-minute refresh) adds newly seen UUIDs as `{}` and never overwrites an existing entry, so hand-written options survive. Missing or empty files are fine — every node then keeps the defaults above. The **Nodes** page of the console edits both options, or use `POST /api/settings/set?type=bot_node_config`.

### Message templates

`controllerMessage` templates are rendered before sending. Available variables (channels with `markdown: true` additionally wrap the verbatim values in Markdown — see the field notes above):

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

### What applies without a restart

Saving settings applies most of them immediately. How each group takes effect:

| Settings | How it applies |
|---|---|
| `controllerMethod` (all five channels) | Every channel is **rebuilt**: the running controllers are stopped and a fresh set is built from the new settings. A channel that owns a connection reconnects — Telegram re-runs its `getMe` handshake, OneBot opens a new WebSocket — so notifications sent during the swap are lost. The rebuild only happens when this section actually changed; saving a message template does not disturb the channels. |
| `controllerMessage`, `debug`, `bot_user_config.json`, `bot_node_config.json`, `webhook.endpoints` | Picked up as they are used; nothing is restarted. A flap window or notification switch edited while an `Offline` is held is re-read when the hold fires. |
| `system.debugMode` | Applies to both behavior and log filtering. |
| `system.networkProxy` | Read on every request and every dial, so a new address reaches channels that are already running. Only the per-channel `networkUseProxy` opt-in is fixed when a channel is built, so toggling that still needs the rebuild above. |
| `system.listenAddr` / `listenPort`, `webhook.enabled` / `listenAddr` / `listenPort`, `komari.dashboardURL`, `dataPath`, `dbPath` | **Applied at startup only.** Saving them changes the file and the in-memory configuration but not the running listener, database or Komari client — restart to apply. `/api/settings/set` reports them back in `restartRequired`. |

## API

Success responses follow the envelope `{"success": true, "message": "...", "data": {...}}`, with the payload nested under a single `data` key. Error responses use `{"success": false, "message": "..."}`. `message` may be omitted on success when there is nothing to report.

### Authentication

Requests are authenticated with HTTP headers:

| Header | Meaning |
|---|---|
| `X-Token` | Token returned by login/register. Held in memory only (lost on restart). |
| `X-Timestamp` | Unix timestamp (seconds); rejected if more than ±30 minutes from server time. **Skipped entirely when `system.debugMode` is `true`.** |

Tokens idle for more than 1 hour are expired (cleaned every 10 minutes); any authenticated call refreshes the timer. Permission levels: `None`, `bot`, `admin`. Endpoints requiring `bot` accept both `bot` and `admin` tokens. Currently registration/login always issue `admin`-level tokens.

Browser WebSocket handshakes cannot carry custom headers, so `/api/system/getLogs` also accepts the same credentials as `?token=` and `?timestamp=` query parameters (headers still take precedence when both are present). Only `admin` tokens are accepted; the timestamp is checked once at handshake time, so an accepted connection stays open past its tolerance window. Because the query string can leak into proxy and access logs, a token-carrying WebSocket URL should be treated as a secret.

### Endpoints

| Endpoint | Method | Permission | Description |
|---|---|---|---|
| `/api/user/login` | POST | None | Log in. Body `{username, password}`. Returns `data: {token, userID, username, level, registerDate}`. |
| `/api/user/register` | POST | None | Register the first user. Body `{username, password}`. Only allowed while no user exists; otherwise `403`. Returns `data: {token, userID, username, level}`. |
| `/api/server/list` | GET | bot / admin | List all monitored servers as rendered text (`data.list`, same output as the bot's `/list`). |
| `/api/server/getInfo` | GET | admin | Static server info (mirrors the bot's `/info`). Query `?uuid=<uuid>` (or `all`). Returns `data: {<uuid>: {uuid, name, info}}` — one entry per requested server. `404` for an unknown single uuid. |
| `/api/server/getStatus` | GET | admin | Live server status (mirrors the bot's `/status`). Query `?uuid=<uuid>` (or `all`). Returns `data: {<uuid>: {uuid, name, online, report}}`; `report` is `null` when the node has not reported yet. `404` for an unknown single uuid. |
| `/api/server/exec` | POST | bot / admin | Execute a command. Body `{uuid: [<uuid>...], command}`. Dispatches a Komari task and polls until completion (or timeout). Returns `data: {taskID, results}`. |
| `/api/settings/get` | GET | admin | `?type=global\|bot_user_config\|bot_node_config` — returns `data: {config}`, the selected config file's content (same layout as the JSON file). |
| `/api/settings/set` | POST | admin | `?type=<same types>` + JSON body of partial updates, e.g. `{"system":{"debugMode":true}}` — deep-merges the body into the selected file, persists it, and reloads it in memory. Only the given keys change; arrays replace, and a `null` deletes a key. Returns `data: {type, restartRequired}`, where `restartRequired` lists the keys the update changed that are only read at startup (see [What applies without a restart](#what-applies-without-a-restart)) — the write succeeds regardless, this only says which edits are not live yet. Always an array, empty when everything took effect. |
| `/api/webhook/add` | POST | admin | Add an incoming webhook endpoint. Body `{name, enabled?, token?, notifyPipes?}` — only the fields given are stored, the rest start at their defaults. `409` when the name is already configured. |
| `/api/webhook/modify` | POST | admin | Change an existing endpoint. Body `{name, ...}` — the fields given are the fields that change (same partial-update rule as `/api/settings/set`, but scoped to one endpoint). `404` for an unknown name, `400` when no other field is given. |
| `/api/webhook/delete` | POST | admin | Remove an endpoint. Body `{name}`. `404` for an unknown name. |
| `/api/webhook/list` | GET | admin | Every configured incoming webhook endpoint, keyed by name, under `data.endpoints`. |
| `/health` | GET | None | Health check. Returns `data: {status, database}`. |
| `/api/system/getLogs` | WebSocket | admin | Streams logs. Sends the last 100 buffered entries, then live `{level, content, timestamp}` events. Credentials via `X-Token`/`X-Timestamp` headers or `?token=`/`?timestamp=` query parameters; a failed check answers with the JSON error and no upgrade. |

Middleware applied to the whole server:

- **Rate limit** — token bucket, 100 requests/minute per client IP (`429` when exceeded).
- **CORS** — `Access-Control-Allow-Origin: *`, allows `Content-Type`, `X-Token`, `X-Timestamp`, `Authorization`.
- **Security headers** — `X-XSS-Protection`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, a restrictive CSP.
- **WebSocket auth** — the log endpoint is wrapped in an admin-only auth middleware at route level, so an unauthorized client is rejected before the upgrade.

### Incoming webhook API

A listener of its own, so external applications can be pointed at it without being able to reach the admin API. It is switched on with `webhook.enabled` and binds `webhook.listenAddr:webhook.listenPort` (default `0.0.0.0:8081`); that half of the configuration is applied at startup, while `webhook.endpoints` is re-read whenever the config is reloaded. Only the rate limit and CORS middleware apply here — no session token is involved.

The console's **WebHooks** page manages the listener settings and the endpoints. The outgoing WebHook notification channel (`controllerMethod.webhook`) stays on the Settings page with the other notification channels, since it is one of them.

| Endpoint | Method | Permission | Description |
|---|---|---|---|
| `/api/webhook/post/<name>` | POST | Endpoint token | Relay an alert to the channels the endpoint lists in `notifyPipes`. Body `{token, subject, content}`. Returns `data: {endpoint, channels}`. |

Every entry under `webhook.endpoints` is one endpoint, addressed by its key as the last path segment: the key `example` is served at `POST /api/webhook/post/example`. The `post/` segment keeps the endpoints' own namespace separate from the management routes (`/api/webhook/add` and friends), which live on the admin listener. Endpoints are managed over the admin API, which writes the same `webhook.endpoints` section of `config.json`; a newly added endpoint accepts requests as soon as the configuration is reloaded, without a restart. An endpoint holds:

| Field | Meaning |
|---|---|
| `enabled` | Whether the endpoint accepts requests. A disabled endpoint answers `403`. |
| `token` | Shared secret the caller sends as the `token` body field; compared in constant time. An endpoint with an empty token answers `500` instead of accepting requests from anyone. |
| `notifyPipes` | The channels the alert is delivered to, named as in `controllerMethod`: `qq_onebot`, `telegram`, `email`, `ntfy`, `webhook`. A channel that is unknown or disabled is skipped and reported. |

The management API accepts exactly these three fields. A request naming any other field, or giving one of them the wrong type (`enabled` must be a boolean, `token` a string, `notifyPipes` an array of strings), is refused with `400` instead of being written to `config.json` — a field the program does not understand must not end up in the file. A `name` must be non-empty and free of `/`, since it becomes the last segment of the endpoint URL. Request bodies are capped at 1 MiB.

The alert is rendered per channel as:

```
{{ subject }}
- Source: {{ source }}
- Content:
{{ content }}

- Time: {{ time }}
Sent by Nukumizu Alert System
```

`{{ source }}` is the endpoint name, so recipients can tell which application triggered the alert. On a channel with `markdown: true` the source is wrapped in inline code and the content in a fenced code block; `{{ subject }}` and `{{ time }}` stay plain.

Status codes: `200` delivered, `400` malformed body or empty `subject`/`content`, `401` wrong token, `403` endpoint disabled, `404` unknown endpoint name, `405` non-POST request, `500` endpoint has no token configured, `502` no channel accepted the alert.

## Bots

QQ (OneBot 11) and Telegram bots share one command engine and authorization pipeline, implemented in `internal/controller/`. The QQ controller speaks OneBot 11 over WebSocket (event stream + actions), so it works with NapCat or any compatible implementation; Telegram uses `go-telegram/bot` long polling.

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

QQ and Telegram are *interactive* channels. Email, ntfy, and webhook are **status-only** channels — they receive status-change alerts but cannot run commands. On startup, the welcome message and initial server list are delivered only to the bot channels (QQ / Telegram), honoring each member's `event_bot_started` preference.

All five channels can also carry an alert submitted by an external application through the [incoming webhook API](#incoming-webhook-api). A bot channel delivers it to the groups and admins configured for that channel; a status-only channel delivers it to its configured destination (mail recipients, ntfy topic, outgoing webhook URL). Markdown formatting is decided per channel by its `markdown` setting, never by the channel's name.

Status-change notifications pass through the per-node gates in `bot_node_config.json` first (`enableStatusNotify`, `flap_tolerant`), so both chat bots and status-only channels see exactly the same filtered stream.

## Building

Requires Go 1.27.3+ and — to build the web console — Node.js 22+.

Helper scripts in the repo root build the Vue console first, then compile the backend with it embedded. They also bake the current git commit and build time into the binary via `-ldflags`. The name states the **target** platform, and each target has a Windows (`.bat`) and a Linux/macOS (`.sh`) flavor: run the flavor for the host you are building on, since every script cross-compiles to its target.

| Script | Output |
|---|---|
| `build-linux-x86_64.sh` / `build-linux-x86_64.bat` | `nukumizu-linux-amd64` |
| `build-win-x86_64.sh` / `build-win-x86_64.bat` | `nukumizu-windows-amd64.exe` |

```bash
# Linux / macOS
./build-linux-x86_64.sh
./build-win-x86_64.sh

# Windows
build-linux-x86_64.bat
build-win-x86_64.bat
```

The console is **embedded in the binary**. Vite writes it to `web/dist` and `web/embed.go` compiles that directory in with `go:embed`, so the executable serves the whole frontend on its own — copy it anywhere, with neither `frontend/` nor `web/dist` next to it, and `/` still returns the console. The build scripts run `npm ci` when `frontend/node_modules` is missing and `npm run build` on every run, so they need Node.js 22+ on the build machine (not on the machine that runs the binary).

Building the backend therefore requires the console to have been built at least once: `web/dist` is a generated, gitignored directory, and `go build` fails with `pattern all:dist: no matching files found` until it exists. Any `build-*` script handles that ordering for you.

Equivalent manual builds:

```bash
# 1. Console (once per frontend change)
cd frontend && npm ci && npm run build && cd ..

# 2. Backend
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

CI (`.github/workflows/build.yml`) runs both flavors — `.sh` on `ubuntu-latest`, `.bat` on `windows-latest` — and uploads the two self-contained binaries as artifacts.

## Running

All three config files are read from the working directory; there are no path flags:

```bash
# Development (go run, so the config files must be in the CWD)
./run.sh          # Linux / macOS
run.bat           # Windows

# Or build first, then run the binary
./nukumizu-linux-amd64
```

`run.sh` / `run.bat` only run the Go backend — they do not build the console, so `web/dist` must already exist (run any `build-*` script once, or `npm run build` in `frontend/`) or the build fails.

On startup the program logs in to Komari, loads node state, connects the status WebSocket, then starts each enabled controller and the HTTP server on `listenAddr:listenPort`. Press `Ctrl+C` for a graceful shutdown. The incoming webhook listener starts separately when `webhook.enabled` is `true`; a failure to bind it is logged without stopping the rest of the program.

### Frontend development

While working on the frontend, run the two sides separately:

```bash
# Terminal 1 — backend (API + WebSocket) on :8080
./run.sh

# Terminal 2 — Vite dev server with hot reload on :5173
cd frontend
npm install
npm run dev
```

Open http://localhost:5173. The dev server proxies `/api` — including the log WebSocket — to `http://127.0.0.1:8080`; point it elsewhere with `NUKUMIZU_API` if the backend listens on another address:

```bash
NUKUMIZU_API=http://192.168.1.10:8080 npm run dev
```

For a production build the Go server serves the embedded console itself, on the normal listen address — see [Building](#building). Vite is not involved at runtime, so `npm run build` alone does not change what a running binary serves: rebuild the binary to pick up frontend changes.

## Testing

```bash
go test ./...
```

The suite runs against the config, handler, node-tracker, controller and pipe packages. It needs no Komari instance or network access.

## Changelog

Release notes and the commits behind each version live in [changeLogs.md](changeLogs.md).

## License

See [LICENSE](LICENSE).
