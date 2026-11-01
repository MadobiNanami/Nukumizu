# Nukumizu 后端

[English](README.md) | 简体中文

面向 [Komari](https://www.komari.wiki) 的远程服务器监控与命令执行子系统。

Nukumizu 会连接 Komari 面板，在内存中维护所有被监控服务器的状态，并把告警推送到多个通知渠道。同时它还在 QQ（通过 [NapCat](https://napneko.github.io/) 或其他 OneBot 11 实现）和 Telegram 上运行交互式机器人，让你可以直接在聊天里控制服务器。前端控制台（Vue 3）编译后内嵌进二进制，浏览器里也能完成同样的监控与配置。

## 功能

- **Komari 集成** — 登录 Komari 面板，启动时及之后每 5 分钟刷新节点列表，并通过 Komari WebSocket 每 5 秒拉取一次各节点最新状态。
- **上游请求重试** — 登录与节点获取在遇到传输层错误、`429` 或 `5xx` 时最多重试 6 次，采用线性退避（0.5s × 第几次）。凭据错误、`4xx` 响应和 JSON-RPC 错误会立即失败，不会反复冲击面板。
- **状态跟踪** — 线程安全的节点跟踪器保存每个服务器的最新报告与静态信息，并检测 `Online` / `Offline` 状态切换。启动后的第一份快照被视为基线，因此重启不会产生一堆误报的“离线”告警。
- **单节点通知控制** — `bot_node_config.json` 可以单独关闭某个节点的状态推送，也可以把 `Offline` 通知暂缓一段可配置的时间：节点在窗口内恢复时，这条告警完全不会发出（抖动抑制）。
- **远程命令执行** — 通过 Komari 任务接口下发命令，并轮询结果（每秒一次，最长 60 秒超时）。
- **交互式机器人** — QQ（OneBot 11，例如 NapCat）与 Telegram 机器人，支持 `/list`、`/status`、`/info`、`/run`、`/shutdown`、`/reboot` 等命令，并带有管理员 / 可信群组的权限模型。
- **通知渠道** — 状态变更会推送到所有已启用的渠道：QQ、Telegram、邮件（SMTP）、[ntfy](https://ntfy.sh) 和 Webhook。
- **入站 Webhook API** — 外部应用可以通过 `POST /api/webhook/post/<name>` 推送自己的告警，Nukumizu 会转发给该端点配置的渠道。每个端点有独立的 token 和目标渠道，且该 API 运行在**独立监听端口**上，可以只对外开放它而不暴露管理 API。
- **网络代理** — 可配置全局代理地址，并在每个渠道上通过 `networkUseProxy` 单独开启；支持 HTTP、WebSocket，甚至 SMTP（通过 HTTP CONNECT 隧道）。
- **可自定义的消息模板** — 所有机器人 / 通知消息都由 `config.json` 中的模板渲染，Markdown 格式按渠道单独开关。
- **热重载** — 保存设置后绝大部分配置立即生效；少数只在启动时读取的键会通过 `restartRequired` 返回给调用方。
- **存储** — `user.db` 与 `log.db` 使用 SQLite（纯 Go 驱动）；采用回滚日志而非 WAL，避免在网络共享盘上损坏数据库。
- **管理 API** — 基于 token 的 REST API，以及仅管理员可用的实时日志 WebSocket。
- **Web 控制台** — Vue 3 管理界面，可浏览节点及其单节点选项、编辑设置、管理机器人信任与 Webhook 端点、实时查看日志。构建产物内嵌在二进制中，单个可执行文件即可同时提供 API 与控制台。
- **测试** — 各 Go 包带有单元测试（`go test ./...`），覆盖重试、抖动抑制和设置合并等路径。

## 工作原理

1. 启动时 Nukumizu 加载 `config.json`、`bot_user_config.json` 和（可选的）`bot_node_config.json`，然后登录 Komari 面板。登录遇到传输错误 / `429` / `5xx` 时最多重试 6 次；仍然失败则进程退出。
2. 获取节点列表（名称 → UUID、静态信息）并存入内存，随后连接 `/api/rpc2` 的 WebSocket，每 5 秒请求一次最新状态。连接断开后按指数退避重连（1s、2s、4s……最大 30s）；连续 5 次失败会记录错误日志，并继续每 30 秒重试。
3. 节点跟踪器会等待覆盖所有已知节点的首份快照后再启用通知（另有两分钟兜底），并且始终把第一份快照当作启动基线 —— 不会因为启动时的状态而产生告警。
4. 真实的在线状态切换会通过 `SERVER_STATUS_CHANGED` 模板渲染，并发送给所有已启用的渠道。在此之前会先经过 `bot_node_config.json` 的单节点开关：`enableStatusNotify` 可以直接丢弃这次变更，`flap_tolerant` 会把 `Offline` 暂缓 N 秒 —— 节点在窗口内恢复时，`Offline` 和随后的 `Online` 都不会发送。
5. 首次状态刷新完成后，QQ / Telegram 还会收到启动欢迎消息和初始服务器列表，并遵循每个成员的 `event_bot_started` 偏好。
6. 每 12 小时重新登录一次 Komari；节点列表每 5 分钟刷新一次。

## 项目结构

```
nukumizu-backend/
├── main.go                       # 入口、启动流程、后台任务、优雅退出
├── router.go                     # HTTP 路由注册（主监听 + webhook 监听）
├── config/
│   ├── config.go                 # 加载配置文件、填充默认值、单节点选项读取
│   ├── variables.go              # 配置结构体 + 原子配置单例
│   ├── settings.go               # 设置文件读取/合并/写入、启动项检测
│   ├── webhook.go                # 入站 Webhook 端点增删改辅助函数
│   ├── reload.go                 # 重载钩子（日志级别、渠道重建）
│   └── *_test.go                 # 设置合并、并发与抖动抑制测试
├── global/
│   └── variables.go              # 构建元信息（名称/版本/开发者）+ 配置路径
├── handler/
│   ├── user.go                   # /api/user/login、/api/user/register
│   ├── server.go                 # /api/server/list、getInfo、getStatus、exec
│   ├── settings.go               # /api/settings/get、set
│   ├── webhook.go                # /api/webhook/post/{name}（入站 Webhook API）
│   ├── webhook_endpoints.go      # /api/webhook/add、modify、delete、list
│   └── health.go                 # /health
├── database/
│   └── user.go                   # user.db（SQLite）用户存储
├── utils/
│   ├── auth.go                   # Token 管理、鉴权中间件、JSON 响应
│   └── middleware.go             # 限流、CORS、安全响应头
├── postLog/                      # 日志子系统
│   ├── postLog.go                # 分级日志（标准输出 + 广播）
│   ├── database.go               # log.db（SQLite，每次运行一张表）
│   ├── logBroadcaster.go         # 向 WebSocket 客户端分发
│   └── logSocketHandler.go       # /api/system/getLogs WebSocket（仅管理员）
├── web/
│   ├── embed.go                  # 将构建好的控制台（web/dist）嵌入二进制
│   ├── dist/                     # Vite 构建输出 —— 生成物，已 gitignore
│   └── handler.go                # 静态文件服务 + SPA 回退
├── internal/
│   ├── komari/
│   │   ├── client.go             # Komari HTTP/JSON-RPC 客户端（登录、节点、任务执行/轮询）
│   │   └── ws.go                 # Komari 状态 WebSocket（轮询 + 重连）
│   ├── node/
│   │   └── tracker.go            # 线程安全节点状态、基线门控、状态切换
│   ├── netproxy/
│   │   └── netproxy.go           # 各渠道统一网络代理
│   ├── template/
│   │   └── template.go           # 消息模板渲染（{{ 变量 }}）
│   └── controller/
│       ├── controller.go         # Manager、接口、告警路由、抖动抑制
│       ├── trigger.go            # 命令解析、鉴权、路由
│       ├── processor.go          # 命令处理
│       ├── utils.go
│       └── pipes/
│           ├── email.go          # 邮件通知渠道
│           ├── ntfy.go           # ntfy 通知渠道
│           ├── webhook.go        # 出站 Webhook 通知渠道
│           ├── qq_onebot/
│           │   ├── qq.go         # QQ 渠道（OneBot 11，例如 NapCat）
│           │   └── onebot.go     # OneBot 11 WebSocket 客户端（事件 + API）
│           └── telegram/
│               ├── telegram.go   # Telegram 机器人（go-telegram/bot，长轮询）
│               └── send.go       # 消息发送 / 拆分（Telegram Markdown）
└── frontend/                     # Vue 3 管理控制台（Vite）
    ├── index.html
    ├── vite.config.js            # 开发服务器；把 /api 代理到后端
    └── src/
        ├── main.js               # 启动：主题 + 运行时开关
        ├── App.vue               # 根组件 + toast 容器
        ├── api/index.js          # REST 接口封装
        ├── router/index.js       # 路由与登录守卫
        ├── styles/               # theme.css + ui.css
        ├── utils/                # http/auth/theme/toast/format/runtime 辅助
        ├── components/           # Modal、Toggle、编辑器、ConfigSection、顶栏、侧栏
        └── views/                # Login、Layout、Overview、Trusted、Settings、WebHooks、Logs
```

## 环境要求

- Go **1.27.3** 或更高版本（见 `go.mod`）
- [Node.js](https://nodejs.org) **22** 或更高版本 —— 仅在构建 Web 控制台时需要，预编译的二进制不需要
- 一台可从本机访问的 [Komari](https://www.komari.wiki) 面板
- QQ：一个提供正向 WebSocket 端点的 OneBot 11 实现，例如 [NapCat](https://napneko.github.io/)（`/api` 与 `/event`）
- Telegram：来自 [@BotFather](https://t.me/BotFather) 的 bot token

主要依赖：`github.com/go-telegram/bot`、`github.com/gorilla/websocket`、`gopkg.in/mail.v2`、`modernc.org/sqlite`。

## 配置

程序从工作目录读取三个 JSON 文件。**没有命令行参数**：文件名固定为 `config.json`、`bot_user_config.json`、`bot_node_config.json`，因此请在存放这些文件的目录下启动程序。

| 文件 | 是否必需 | 用途 |
|---|---|---|
| `config.json` | 是 | 核心配置：系统、调试开关、Komari、Webhook 监听、渠道、消息模板、存储路径 |
| `bot_user_config.json` | 是 | 各机器人的管理员 / 可信群组及其通知偏好 |
| `bot_node_config.json` | 否 | 单节点选项（通知开关、抖动抑制窗口）。首次获取节点时自动创建 |

> 三个文件都在 `.gitignore` 中，因为它们包含凭据（Komari 密码、bot token、代理认证）。请参考下面的示例，切勿提交真实密钥。`bot_node_config.json` 本身不含敏感信息，但也一并被忽略。

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

字段说明：

- `controllerMethod` 以渠道名为键：`qq_onebot`、`telegram`、`email`、`ntfy`、`webhook`。同样的名称也用于 `notifyPipes` 和 `bot_user_config.json`。QQ 渠道使用 OneBot 11 协议，`oneBotAddr` / `oneBotPort` / `oneBotToken` 指向 OneBot 实现（NapCat 默认在 `127.0.0.1:3000`）。
- `system.networkProxy` 是**全局**代理地址。只有当渠道自身的 `networkUseProxy` 为 `true` 时才会使用它，作用于 Telegram HTTP 轮询、QQ OneBot WebSocket、ntfy 与 Webhook 请求，以及邮件 SMTP（通过 HTTP CONNECT 隧道）。
- `webhook` 配置的是**入站** Webhook API（见 [入站 Webhook API](#入站-webhook-api)）；`controllerMethod.webhook` 配置的是出站 Webhook 通知渠道。两者互不相关。
- `markdown` 是五个渠道各自独立的开关。为 `false`（默认）时所有渲染值都以纯文本插入；为 `true` 时，需要原样阅读的值（UUID、事件消息、命令、命令结果、告警来源与内容）会被包裹为 Markdown 行内代码 / 代码块。程序不会根据渠道名推断，因此渠道只会得到你明确要求的格式 —— 平台不支持 Markdown 时请关闭它。在 Telegram 上它同时决定 `parse_mode`：关闭 `markdown` 时不发送 `parse_mode`，含 `*` 或 `_` 的文本会原样送达，而不会被 API 以“Markdown 格式错误”拒绝。
- `controllerMethod.qq_onebot.listenMethod` / `telegram.listenMethod` —— 见 [机器人响应模式](#机器人响应模式)。
- `debug` 控制各模块的详细日志，仅在 `system.debugMode` 为 `true` 时生效：
  - `showOneBotMsg` / `showOneBotAction` —— 收到的 OneBot 消息 / 发出的 OneBot API 调用。
  - `showTelegramMsg` —— 收到的 Telegram 更新。
  - `showTriggerCmdEcho` —— 交给触发器解析后的命令。
  - `showKomariTaskEcho` —— Komari 任务的创建与轮询。
  - `oneBotIgnoreSelfMsg` —— 忽略 OneBot 回显的机器人自身消息。
  - `showFlapTolerantEcho` —— 抖动抑制的决策（暂缓 / 释放 / 补发）。
- `email.useTLS` 仅为配置兼容而保留。
- `dbPath` 默认为 `./db`，`user.db` 与 `log.db` 会创建在其中。`dataPath` 默认为 `./data`，目前尚未使用，预留给后续的文件存储。
- 缺失的键会使用内置默认值（地址 `0.0.0.0`、端口 `8080`、Webhook API `0.0.0.0:8081`、无 Webhook 端点、OneBot `127.0.0.1:3000`、ntfy 服务器 `https://ntfy.sh`、Webhook 方法 `POST` 等）。消息模板也有内置默认值。`markdown` 默认为 `false`，需要 Telegram 格式时请像示例那样显式开启。

### `bot_user_config.json`

管理员与可信群组按**机器人渠道**分别定义，并把成员 ID 映射到该成员的通知偏好：

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

- **QQ**：成员 ID 为 QQ 号；群 ID 为数字群号。
- **Telegram**：成员 ID 可以是 `@username`（该用户给机器人发过消息后会解析为数字用户 ID）或数字用户 ID；群 ID 为数字聊天 ID（超级群为负数）。
- 每个成员的选项：
  - `event_status_notify` —— 接收服务器 `Online`/`Offline` 推送。
  - `event_bot_started` —— 接收启动时的自动消息（欢迎语 + 初始服务器列表）。
  - `event_reply` —— 预留，用于关闭对该成员命令的自动回复。
- 某渠道的管理员也会出现在机器人发送通知的所有可信群组 / 私聊上下文中。

### `bot_node_config.json`

单节点选项，以 Komari 上报的节点 UUID 为键：

```json
{
    "0f2a9c1e-1234-4a5b-8c9d-000000000001": {
        "enableStatusNotify": false,
        "flap_tolerant": 30
    },
    "7d3b8f20-5678-4c9d-9e0f-000000000002": {}
}
```

| 字段 | 默认值 | 含义 |
|---|---|---|
| `enableStatusNotify` | `true` | 为 `false` 时该节点的状态变更**不会**推送 —— 聊天机器人和通知渠道都不会发送。省略该字段（或整个条目）即为开启。 |
| `flap_tolerant` | `0` | `Offline` 通知的暂缓秒数（允许小数，上限 86400）。节点在窗口内恢复时，`Offline` 与 `Online` 都会被抑制；窗口结束时仍未恢复则补发 `Offline`。为 `0` 或省略时立即通知。 |

该文件由程序自动维护：每次获取节点（`FetchNodes` —— 启动、重连和每 5 分钟刷新）都会把新出现的 UUID 以 `{}` 形式加入，且从不覆盖已有条目，因此手写的选项不会被冲掉。文件缺失或为空也没问题 —— 所有节点使用上述默认值。可通过控制台的 **Nodes** 页面修改这两项，或调用 `POST /api/settings/set?type=bot_node_config`。

### 消息模板

`controllerMessage` 中的模板会在发送前渲染。可用变量如下（`markdown: true` 的渠道还会对需要原样阅读的值加 Markdown 包裹，见上文）：

| 变量 | 含义 |
|---|---|
| `{{ time }}` | 服务器当前时间 |
| `{{ serverName }}` | 目标服务器名称 |
| `{{ serverUUID }}` | 目标服务器 UUID |
| `{{ upStatus }}` | `Online` / `Offline` |
| `{{ event }}` | 状态变更事件（`Online` / `Offline`） |
| `{{ message }}` | 状态事件附带的消息 |
| `{{ command }}` | 被执行的命令 |
| `{{ result }}` | 命令执行结果 |
| `{{ list.onlineServers }}` | 格式化后的在线服务器列表（`- 名称 (uuid)`） |
| `{{ list.offlineServers }}` | 格式化后的离线服务器列表 |
| `{{ softwareVersion }}`、`{{ softwareBuildVer }}`、`{{ softwareCommitHash }}`、`{{ softwareBuildType }}`、`{{ softwareBuildTime }}`、`{{ softwareDeveloper }}`、`{{ softwareDescription }}` | 构建元信息（commit hash 与构建时间在编译期注入） |

### 无需重启即可生效的配置

保存设置后，大部分配置立即生效：

| 配置 | 生效方式 |
|---|---|
| `controllerMethod`（全部五个渠道） | 所有渠道会被**重建**：停止正在运行的控制器，再按新设置构建一套。持有连接的渠道会重新连接 —— Telegram 重新执行 `getMe` 握手，OneBot 重新建立 WebSocket —— 因此切换期间的推送会丢失。仅当该配置节确实发生变化时才重建；只保存消息模板不会影响渠道。 |
| `controllerMessage`、`debug`、`bot_user_config.json`、`bot_node_config.json`、`webhook.endpoints` | 使用时即时读取，不会重启任何东西。若在 `Offline` 暂缓期间修改通知开关或抖动窗口，补发时会重新读取。 |
| `system.debugMode` | 同时影响行为与日志过滤。 |
| `system.networkProxy` | 每次请求、每次拨号都会重新读取，因此新地址对已在运行的渠道也生效。只有渠道级的 `networkUseProxy` 在构建渠道时固定，切换它仍需上面的重建。 |
| `system.listenAddr` / `listenPort`、`webhook.enabled` / `listenAddr` / `listenPort`、`komari.dashboardURL`、`dataPath`、`dbPath` | **仅在启动时生效。** 保存会写入文件并更新内存配置，但不会改变正在运行的监听、数据库或 Komari 客户端 —— 需要重启。`/api/settings/set` 会通过 `restartRequired` 返回这些键。 |

## API

成功响应统一为 `{"success": true, "message": "...", "data": {...}}`，负载放在单个 `data` 键下。错误响应为 `{"success": false, "message": "..."}`。成功且无额外信息时 `message` 可以省略。

### 鉴权

请求通过 HTTP 头鉴权：

| 请求头 | 含义 |
|---|---|
| `X-Token` | 登录/注册返回的 token。仅保存在内存中（重启后失效）。 |
| `X-Timestamp` | Unix 时间戳（秒）；与服务器时间相差超过 ±30 分钟会被拒绝。**当 `system.debugMode` 为 `true` 时完全跳过检查。** |

空闲超过 1 小时的 token 会失效（每 10 分钟清理一次）；任何已鉴权的调用都会刷新计时。权限级别：`None`、`bot`、`admin`。要求 `bot` 的接口接受 `bot` 与 `admin` 两种 token。目前注册/登录一律签发 `admin` 级别 token。

浏览器的 WebSocket 握手无法携带自定义请求头，因此 `/api/system/getLogs` 也接受 `?token=` 与 `?timestamp=` 查询参数（两者都存在时请求头优先）。该接口仅接受 `admin` token；时间戳只在握手时检查一次，已建立的连接不受后续容差窗口影响。查询串可能进入代理与访问日志，带 token 的 WebSocket URL 应视为机密。

### 接口列表

| 接口 | 方法 | 权限 | 说明 |
|---|---|---|---|
| `/api/user/login` | POST | None | 登录。请求体 `{username, password}`。返回 `data: {token, userID, username, level, registerDate}`。 |
| `/api/user/register` | POST | None | 注册首个用户。请求体 `{username, password}`。仅在尚无用户时允许，否则 `403`。返回 `data: {token, userID, username, level}`。 |
| `/api/server/list` | GET | bot / admin | 以渲染后的文本列出所有被监控服务器（`data.list`，与机器人 `/list` 输出一致）。 |
| `/api/server/getInfo` | GET | admin | 服务器静态信息（对应机器人 `/info`）。查询 `?uuid=<uuid>`（或 `all`）。返回 `data: {<uuid>: {uuid, name, info}}`，每个请求的服务器一项。单个未知 uuid 返回 `404`。 |
| `/api/server/getStatus` | GET | admin | 服务器实时状态（对应机器人 `/status`）。查询 `?uuid=<uuid>`（或 `all`）。返回 `data: {<uuid>: {uuid, name, online, report}}`；节点尚未上报时 `report` 为 `null`。单个未知 uuid 返回 `404`。 |
| `/api/server/exec` | POST | bot / admin | 执行命令。请求体 `{uuid: [<uuid>...], command}`。下发 Komari 任务并轮询至完成（或超时）。返回 `data: {taskID, results}`。 |
| `/api/settings/get` | GET | admin | `?type=global\|bot_user_config\|bot_node_config` —— 返回 `data: {config}`，即对应配置文件的内容（布局与文件一致）。 |
| `/api/settings/set` | POST | admin | `?type=<同上>` + 部分更新的 JSON 请求体，例如 `{"system":{"debugMode":true}}` —— 深度合并到对应文件、写盘并在内存中重载。只改动给出的键；数组整体替换，`null` 删除该键。返回 `data: {type, restartRequired}`，其中 `restartRequired` 列出本次改动中只在启动时读取的键（见 [无需重启即可生效的配置](#无需重启即可生效的配置)）—— 写入始终成功，这个字段只表示哪些修改尚未生效。始终为数组，全部生效时为空。 |
| `/api/webhook/add` | POST | admin | 新增入站 Webhook 端点。请求体 `{name, enabled?, token?, notifyPipes?}` —— 只保存给出的字段，其余使用默认值。名称已存在时返回 `409`。 |
| `/api/webhook/modify` | POST | admin | 修改已有端点。请求体 `{name, ...}` —— 给出哪些字段就改哪些（规则同 `/api/settings/set`，但限定在单个端点）。名称不存在返回 `404`，未给出其他字段返回 `400`。 |
| `/api/webhook/delete` | POST | admin | 删除端点。请求体 `{name}`。名称不存在返回 `404`。 |
| `/api/webhook/list` | GET | admin | 所有已配置的入站 Webhook 端点，以名称为键，位于 `data.endpoints`。 |
| `/health` | GET | None | 健康检查。返回 `data: {status, database}`。 |
| `/api/system/getLogs` | WebSocket | admin | 推送日志。先发送最近 100 条缓冲，再实时推送 `{level, content, timestamp}`。凭据可通过 `X-Token`/`X-Timestamp` 请求头或 `?token=`/`?timestamp=` 查询参数提供；校验失败会返回 JSON 错误且不升级连接。 |

作用于整个服务的中间件：

- **限流** —— 令牌桶，每个客户端 IP 每分钟 100 次请求（超出返回 `429`）。
- **CORS** —— `Access-Control-Allow-Origin: *`，允许 `Content-Type`、`X-Token`、`X-Timestamp`、`Authorization`。
- **安全响应头** —— `X-XSS-Protection`、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy` 以及严格的 CSP。
- **WebSocket 鉴权** —— 日志端点通过路由级中间件要求管理员 token，未授权客户端在升级前即被拒绝。

### 入站 Webhook API

它使用独立监听端口，外部应用可以只访问它而接触不到管理 API。通过 `webhook.enabled` 开启，监听 `webhook.listenAddr:webhook.listenPort`（默认 `0.0.0.0:8081`）；这部分配置在启动时生效，而 `webhook.endpoints` 会在配置重载时重新读取。这里只应用限流与 CORS 中间件，不涉及会话 token。

控制台的 **WebHooks** 页面负责监听设置与端点管理。出站 Webhook 通知渠道（`controllerMethod.webhook`）仍位于设置页，与其他通知渠道放在一起，因为它本身就是其中一个渠道。

| 接口 | 方法 | 权限 | 说明 |
|---|---|---|---|
| `/api/webhook/post/<name>` | POST | 端点 token | 把告警转发给该端点 `notifyPipes` 中列出的渠道。请求体 `{token, subject, content}`。返回 `data: {endpoint, channels}`。 |

`webhook.endpoints` 下的每个条目就是一个端点，以键名作为 URL 最后一段：键 `example` 对应 `POST /api/webhook/post/example`。`post/` 这一段把端点命名空间与管理路由（`/api/webhook/add` 等，位于管理监听）区分开。端点通过管理 API 增删改，写入的正是 `config.json` 的 `webhook.endpoints` 部分；新增端点在配置重载后即可接收请求，无需重启。端点包含：

| 字段 | 含义 |
|---|---|
| `enabled` | 是否接受请求。禁用时返回 `403`。 |
| `token` | 调用方在请求体 `token` 字段中提供的共享密钥，使用常量时间比较。未配置 token 的端点返回 `500`，而不是对任何人开放。 |
| `notifyPipes` | 告警投递的渠道，命名与 `controllerMethod` 一致：`qq_onebot`、`telegram`、`email`、`ntfy`、`webhook`。未知或未启用的渠道会被跳过并上报。 |

管理 API 只接受上述三个字段。出现其他字段，或字段类型错误（`enabled` 必须是布尔值、`token` 必须是字符串、`notifyPipes` 必须是字符串数组）时返回 `400`，不会写入 `config.json` —— 程序不认识的字段不应出现在文件里。`name` 不能为空且不能包含 `/`，因为它会成为端点 URL 的最后一段。请求体上限为 1 MiB。

告警按渠道渲染为：

```
{{ subject }}
- Source: {{ source }}
- Content:
{{ content }}

- Time: {{ time }}
Sent by Nukumizu Alert System
```

`{{ source }}` 是端点名称，接收方据此判断是哪个应用触发的告警。`markdown: true` 的渠道会把 source 包裹为行内代码、content 包裹为代码块；`{{ subject }}` 与 `{{ time }}` 保持纯文本。

状态码：`200` 已投递，`400` 请求体格式错误或 `subject`/`content` 为空，`401` token 错误，`403` 端点已禁用，`404` 端点名称不存在，`405` 非 POST 请求，`500` 端点未配置 token，`502` 没有任何渠道接受该告警。

## 机器人

QQ（OneBot 11）与 Telegram 机器人共用同一套命令引擎与鉴权流程，实现在 `internal/controller/`。QQ 渠道通过 WebSocket 使用 OneBot 11（事件流 + API），因此兼容 NapCat 及同类实现；Telegram 使用 `go-telegram/bot` 长轮询。

### 权限模型

- **可信群组**：*群内*发出的命令只有在该群位于 `trustedGroups` 中时才会得到响应，其他群的消息会被忽略。
- **管理命令**：`/shutdown`、`/reboot` 和 `/run` 还要求*发送者*位于 `admins` 中。
- **私聊**：非管理命令（`/help`、`/list`、`/status`、`/info`、`/getip`）对任何私聊发送者都响应；管理命令仍要求管理员。

### 机器人响应模式

- `global` —— 机器人监听可信群组中的所有消息，无需被 @ 即可响应已知命令；未知的 `/` 命令会被静默忽略。
- `at` —— 机器人只在自己被提及时响应（QQ `@`、Telegram `@botname`）。此模式下未知命令会回复 `Unknown command: /…`。

### 命令

| 命令 | 权限 | 说明 |
|---|---|---|
| `/help` | 所有人 | 显示帮助消息（`BOT_HELP` 模板）。 |
| `/list` | 所有人 | 列出所有服务器及其在线状态（`SERVER_LIST` 模板）。 |
| `/status <uuid>` | 所有人 | 服务器实时报告（CPU、内存、磁盘、网络、运行时长、进程）。 |
| `/info <uuid>` | 所有人 | 服务器静态信息（系统、内核、CPU、内存、交换、磁盘、计费、标签）。 |
| `/getip <uuid>` | 所有人 | 服务器的 IPv4 / IPv6 地址。 |
| `/shutdown <uuid>` | 管理员 | 通过 Komari 任务接口关机。 |
| `/reboot <uuid>` | 管理员 | 通过 Komari 任务接口重启。 |
| `/run <uuid\|all> <command>` | 管理员 | 在单台（`all` 表示全部）服务器上执行命令并回报结果（`SERVER_EXECUTE_RESULT` 模板）。 |
| `/start` | 所有人 | 仅 Telegram —— 发送 `TG_BOT_START` 欢迎消息。 |

## 通知渠道

QQ 与 Telegram 是*交互式*渠道。邮件、ntfy 与 Webhook 是**仅状态**渠道 —— 它们只接收状态变更告警，不能执行命令。启动时，欢迎消息与初始服务器列表只发送给机器人渠道（QQ / Telegram），并遵循每个成员的 `event_bot_started` 偏好。

五个渠道也都能承载外部应用通过[入站 Webhook API](#入站-webhook-api)提交的告警。机器人渠道会发给该渠道配置的群组与管理员；仅状态渠道会发给各自的目的地（收件人、ntfy topic、出站 Webhook URL）。Markdown 格式由各渠道的 `markdown` 设置决定，与渠道名称无关。

状态变更通知会先经过 `bot_node_config.json` 的单节点开关（`enableStatusNotify`、`flap_tolerant`），因此聊天机器人与仅状态渠道看到的是同一份过滤后的数据流。

## 构建

需要 Go 1.27.3+，构建 Web 控制台时还需要 Node.js 22+。

仓库根目录的辅助脚本会先构建 Vue 控制台，再把后端与它一起编译；同时通过 `-ldflags` 把当前 git commit 与构建时间写进二进制。脚本名表示**目标平台**，每个目标都有 Windows（`.bat`）和 Linux/macOS（`.sh`）两种版本：请在当前宿主机上运行对应版本，因为所有脚本都会交叉编译到各自的目标平台。

| 脚本 | 产物 |
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

控制台是**内嵌在二进制中**的。Vite 将其输出到 `web/dist`，`web/embed.go` 通过 `go:embed` 把它编译进去，因此可执行文件可以独立提供完整前端 —— 复制到任何位置，即使旁边没有 `frontend/` 或 `web/dist`，访问 `/` 仍会返回控制台。构建脚本会在 `frontend/node_modules` 缺失时执行 `npm ci`，并在每次运行时执行 `npm run build`，因此构建机需要 Node.js 22+（运行二进制的机器不需要）。

也正因如此，编译后端前必须至少构建过一次控制台：`web/dist` 是生成目录且被 gitignore，在它存在之前 `go build` 会报 `pattern all:dist: no matching files found`。任何 `build-*` 脚本都会替你处理好顺序。

等价的手动构建：

```bash
# 1. 控制台（前端改动后执行一次）
cd frontend && npm ci && npm run build && cd ..

# 2. 后端
# Linux / macOS
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags "-X main.CommitHash=$(git rev-parse --short HEAD) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o nukumizu-linux-amd64 .

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -ldflags "-X main.CommitHash=<commit> -X main.BuildTime=<utc-time>" \
  -o nukumizu-windows-amd64.exe .
```

`main.CommitHash` 与 `main.BuildTime` 会出现在日志和 `BOT_STARTED` 消息中。

CI（`.github/workflows/build.yml`）会运行两种版本 —— `.sh` 在 `ubuntu-latest`，`.bat` 在 `windows-latest` —— 并把两个自包含的二进制作为产物上传。

## 运行

三个配置文件都从工作目录读取，没有路径参数：

```bash
# 开发运行（go run，因此配置文件必须在当前目录）
./run.sh          # Linux / macOS
run.bat           # Windows

# 或先构建，再运行二进制
./nukumizu-linux-amd64
```

`run.sh` / `run.bat` 只运行 Go 后端，不会构建控制台，因此 `web/dist` 必须已存在（执行过一次任意 `build-*` 脚本，或在 `frontend/` 中执行 `npm run build`），否则编译会失败。

启动时程序会登录 Komari、加载节点状态、连接状态 WebSocket，然后启动各已启用渠道以及 `listenAddr:listenPort` 上的 HTTP 服务。按 `Ctrl+C` 优雅退出。当 `webhook.enabled` 为 `true` 时，入站 Webhook 监听会单独启动；绑定失败只会记录日志，不会影响程序其余部分。

### 前端开发

开发前端时，请分别运行前后端：

```bash
# 终端 1 —— 后端（API + WebSocket）监听 :8080
./run.sh

# 终端 2 —— Vite 开发服务器，热更新，监听 :5173
cd frontend
npm install
npm run dev
```

打开 http://localhost:5173。开发服务器会把 `/api`（包括日志 WebSocket）代理到 `http://127.0.0.1:8080`；若后端监听在别处，用 `NUKUMIZU_API` 指向它：

```bash
NUKUMIZU_API=http://192.168.1.10:8080 npm run dev
```

生产构建时，Go 服务会在正常监听地址上直接提供内嵌的控制台 —— 见[构建](#构建)。运行时与 Vite 无关，因此单独执行 `npm run build` 不会改变已运行二进制的内容：需要重新构建二进制才能生效。

## 测试

```bash
go test ./...
```

测试覆盖 config、handler、节点跟踪器、控制器与各管道包，不需要 Komari 实例或网络访问。

## 更新日志

每个版本的发布说明及对应 commit 见 [changeLogs.md](changeLogs.md)。

## 许可证

见 [LICENSE](LICENSE)。
