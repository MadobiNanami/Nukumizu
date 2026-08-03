<<<<<<< HEAD
# Nukumizu Backend

Remote server monitoring and command execution subsystem for [Komari](https://www.komari.wiki).

## Overview

Nukumizu connects to a Komari Dashboard instance to:
- Monitor server status in real-time via WebSocket
- Execute commands on remote servers via the Komari task API
- Provide Bot interfaces on QQ (via NapCat) and Telegram for interactive server control
- Send one-way status notifications via Email, Ntfy, and Webhook

## Project Structure

```
nukumizu-backend/
├── main.go                  # Entry point, startup sequence, graceful shutdown
├── router.go                # HTTP route registration
├── config/
│   └── config.go            # Configuration loading, defaults
├── handler/
│   ├── user.go              # User login/register handlers
│   ├── server.go            # Server list/status/exec handlers
│   ├── bot.go               # Bot message receive handler
│   └── health.go            # Health check endpoint
├── database/
│   └── user.go              # SQLite user database
├── utils/
│   ├── auth.go              # Token management, authentication
│   └── middleware.go        # Rate limit, CORS, XSS protection
├── postLog/                 # Logging subsystem
├── internal/
│   ├── komari/
│   │   ├── client.go        # Komari HTTP API client
│   │   └── ws.go            # Komari WebSocket client
│   ├── node/
│   │   └── tracker.go       # Thread-safe node state tracking
│   ├── controller/
│   │   ├── controller.go    # Controller interface & manager
│   │   ├── qq.go            # QQ (Napcat) Bot controller
│   │   ├── telegram.go      # Telegram Bot controller
│   │   ├── email.go         # Email notification controller
│   │   ├── ntfy.go          # Ntfy notification controller
│   │   └── webhook.go       # Webhook notification controller
│   └── template/
│       └── template.go      # Message template engine
```

## Configuration

Copy and modify `config.json` at the project root:

```json
{
    "system": {
        "debugMode": true,
        "listenAddr": "0.0.0.0",
        "listenPort": "8080"
    },
    "komari": {
        "dashboardURL": "http://127.0.0.1:25774",
        "account": {
            "username": "admin",
            "password": "admin"
        }
    },
    "controllerMethod": {
        "qq(napcat)": {
            "enabled": false,
            "url": "http://127.0.0.1:8081",
            "token": "",
            "botQQID": 0,
            "listenMethod": "global",
            "admins": [],
            "trustedGroups": []
        },
        "telegram": {
            "enabled": false,
            "botToken": "",
            "listenMethod": "global",
            "admins": [],
            "trustedGroups": []
        },
        "email": {
            "enabled": false,
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
            "server": "https://ntfy.sh",
            "topic": "",
            "token": "",
            "priority": "default"
        },
        "webhook": {
            "enabled": false,
            "url": "",
            "method": "POST",
            "headers": {},
            "template": ""
        }
    },
    "controllerMessage": {
        "SERVER_STATUS_CHANGED": "Server Status Changed Alert\n{{ serverName }} - {{ upStatus }}\nEvent: {{ event }}\nServer Name: {{ serverName }}\nMessage: {{ message }}\nTime: {{ time }}",
        "SERVER_LIST": "All server list:\nOnline:\n{{ list.onlineServers }}\nOffline:\n{{ list.offlineServers }}",
        "SERVER_EXECUTE_RESULT": "Command execute result:\nServer Name: {{ serverName }}\nCommand: {{ command }}\n***Result***\n\n{{ result }}\n\n************\nTime: {{ time }}"
    }
}
```

## API Endpoints

All API responses follow the format: `{"success": bool, "message": "..."}`

Authentication is via `X-Token` and `X-Timestamp` HTTP headers.

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `/api/user/login` | POST | None | User login |
| `/api/user/register` | POST | None | First-time registration |
| `/api/server/list` | GET | bot/admin | List all servers |
| `/api/server/getStatus` | GET | bot/admin | Get server recent status |
| `/api/server/exec` | POST | bot/admin | Execute command on server(s) |
| `/api/bot/msg/recv` | POST | bot | Receive bot messages (from napcat-bridge) |
| `/health` | GET | None | Health check |
| `/api/system/getLogs` | WS | None | Real-time log streaming |

## Bot Commands

| Command | Permission | Description |
|---|---|---|
| `/list` | None | List all server status |
| `/shutdown <uuid>` | Admin | Shutdown specific server |
| `/reboot <uuid>` | Admin | Reboot specific server |
| `/status <uuid>` | None | Get server detailed status |
| `/run <uuid\|all> <command>` | Admin | Run command on server(s) |

## Building

```bash
go build -o nukumizu-backend .
```

## Running

```bash
./nukumizu-backend -config config.json
```

## License

See [LICENSE](LICENSE).
=======
# nukumizu-backend
>>>>>>> b3a5a5bad12c5ad7119d7b1b41134830233cdf67
