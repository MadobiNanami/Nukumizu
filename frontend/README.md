# Nukumizu Console

Web console for the Nukumizu backend — a remote-server monitor and alert bot. Built with Vue 3 + Vite, no UI framework; styling is hand-rolled on CSS custom properties with a light/dark theme.

## Features

- **Sign in / first-run admin** — login, or create the very first admin while the database is still empty (the backend rejects further registrations once any user exists).
- **Nodes (overview)** — live server cards from the backend status/info APIs: online state, CPU / RAM / disk load, plus the per-node *status notify* switch persisted to `bot_node_config.json`. Searchable, auto-refreshes.
- **Bot trust** — manage QQ (NapCat) and Telegram admins & trusted groups in `bot_user_config.json`: add / remove members and toggle each member's `status notify`, `startup notify`, and `command replies`.
- **Settings** — structured forms for the global `config.json` (system, debug, Komari dashboard, controller methods, message templates, storage paths). Each card saves only its own section.
- **System logs** — live WebSocket log stream (`/api/system/getLogs`) with severity filter chips, search, pause/resume and export.

## Development

```bash
npm install
npm run dev        # http://localhost:5173
```

The dev server proxies `/api` (and the log websocket) to the backend. By default it targets `http://127.0.0.1:8080`; override with:

```bash
# Windows PowerShell
$env:NUKUMIZU_API = "http://192.168.20.4:8080"; npm run dev
```

Production build:

```bash
npm run build      # outputs ../web/dist
npm run preview
```

Vite writes to `../web/dist` rather than `frontend/dist` (see `build.outDir` in `vite.config.js`) because the Go backend embeds that directory into the binary — `go:embed` cannot reach outside the package it sits in, so the output has to live under `web/`. The repo's `build-*` scripts run this build for you and compile the backend afterwards; `npm run build` alone does not change what an already-built binary serves.

## API contract notes

- Auth uses two headers on every request: `X-Token` (from login) and `X-Timestamp` — a **Unix timestamp in seconds** (not milliseconds) with a ±30 min tolerance (skipped when `system.debugMode` is on).
- Token & user are kept in `localStorage`. On a `401` the session is cleared and you are returned to the login page.
- The panel talks to `/api/server/getStatus`, `/api/server/getInfo`, `/api/settings/get`, `/api/settings/set` and `/api/user/login|register`. All management endpoints require an `admin` token.
- `/api/settings/set` is a deep merge: sending a JSON `null` for a key removes it (used when deleting a trusted member).

## Project structure

```
frontend/
├── index.html
├── vite.config.js
└── src/
    ├── main.js / App.vue
    ├── router/            # routes + auth guard
    ├── api/index.js       # authApi, serverApi, settingsApi
    ├── utils/             # auth, http client, theme, toasts, formatting
    ├── styles/            # theme.css (tokens) + ui.css (primitives)
    ├── components/        # TopBar, SideBar, Modal, Toggle, TagsEditor, HeadersEditor
    └── views/             # Login, Layout, Overview, Trusted, Settings, Logs
```

## Caveats

- The log websocket needs an `admin` token, and a browser cannot set headers on a WebSocket handshake, so the token rides in the query string (`/api/system/getLogs?token=…&timestamp=…`). That URL is a credential: it can end up in proxy and access logs, so don't paste it into third-party tools. The view reconnects with a fresh token from `localStorage` on every attempt.
- Registering more than one user is intentionally impossible; the backend only accepts the very first registration.
