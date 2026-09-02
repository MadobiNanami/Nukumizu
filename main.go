package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/database"
	"nukumizu-backend/global"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/internal/controller/pipes"
	"nukumizu-backend/internal/controller/pipes/qq_napcat"
	"nukumizu-backend/internal/controller/pipes/telegram"
	"nukumizu-backend/internal/komari"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/postLog"
	"nukumizu-backend/utils"
)

// CommitHash and BuildTime are set at build time using -ldflags.
var (
	CommitHash string
	BuildTime  string
)

func main() {
	// Set global software info.
	global.SoftwareInfo.CommitHash = CommitHash
	global.SoftwareInfo.BuildTime = BuildTime

	// Parse CLI flags.
	configPath_global := flag.String("config", "config.json", "Path to configuration file")
	configPath_bot_user := flag.String("bot-user-config", "bot_user_config.json", "Path to bot user configuration file")
	flag.Parse()

	// Log startup banner.
	postLog.Info(fmt.Sprintf("%s Ver.%s.%d.%s.%s Developed by %s at %s", global.SoftwareInfo.Name, global.SoftwareInfo.Version, global.SoftwareInfo.BuildVer, global.SoftwareInfo.BuildType, global.SoftwareInfo.CommitHash, global.SoftwareInfo.Developer, global.SoftwareInfo.BuildTime))

	// Load configuration.
	cfg, err := config.LoadGlobalConfig(*configPath_global)
	if err != nil {
		log.Fatalf("Failed to load global config: %v", err)
	}

	_, err = config.LoadBotUserConfig(*configPath_bot_user)
	if err != nil {
		log.Fatalf("Failed to load bot user config: %v", err)
	}

	// Initialize logging.
	postLog.SetDebugMode(cfg.System.DebugMode)
	postLog.InitLogBroadcaster()

	dbPath := cfg.DBPath

	if err := postLog.InitLogsDatabase(fmt.Sprintf("%s/log.db", dbPath)); err != nil {
		postLog.Fatal("Failed to initialize logs database: " + err.Error())
	}

	// Initialize database.
	if err := database.InitUserDB(fmt.Sprintf("%s/user.db", dbPath)); err != nil {
		postLog.Fatal("Failed to initialize user database: " + err.Error())
	}
	defer database.CloseUserDB()

	// Initialize node tracker.
	node.InitTracker()

	// Initialize controller manager.
	controller.InitManager()

	// Initialize rate limiter.
	utils.InitRateLimiter(100, time.Minute)

	// Start token cleaner.
	utils.StartTokenCleaner()

	postLog.Info("Starting Nukumizu server...")

	// --- Login to Komari ---
	komari.InitClient(cfg.Komari.DashboardURL)

	if err := komari.LoginAndStart(); err != nil {
		postLog.Fatal("Failed to login to Komari: " + err.Error())
		return
	}

	// --- Start Komari WebSocket connection ---
	komari.InitWSClient(cfg.Komari.DashboardURL, 5)
	wsClient := komari.GetWSClient()
	if wsClient != nil {
		wsClient.SetOnReconnectFail(func() {
			postLog.Error("Komari WebSocket reconnection exhausted")
			controller.GetManager().NotifyAllAdmins("Komari WebSocket connection lost after 5 retry attempts")
		})

		if err := wsClient.Connect(); err != nil {
			postLog.Error("Failed to connect Komari WebSocket: " + err.Error())
		}
	}

	// --- Initialize controllers ---
	initControllers()

	// Send the bot initialization message to all enabled bot controllers
	// (QQ_NapCat, Telegram), then the server list right after it once the
	// initial status refresh has completed so the list reflects the real
	// online/offline state.
	if mgr := controller.GetManager(); mgr != nil {
		mgr.ShowBotInitMessage()

		if tracker := node.GetTracker(); tracker != nil {
			tracker.OnBootstrapDone(func() {
				if m := controller.GetManager(); m != nil {
					m.ShowBotServerList()
				}
			})
		}
	}

	// --- Start background tasks ---
	startBackgroundTasks(cfg)

	// --- Setup HTTP router ---
	mux := SetupRouter()

	// Apply middleware chain.
	handler := utils.RateLimitMiddleware(mux)
	handler = utils.CORSMiddleware(handler)
	handler = utils.XSSProtectionMiddleware(handler)

	addr := fmt.Sprintf("%s:%s", cfg.System.ListenAddr, cfg.System.ListenPort)
	postLog.Info(fmt.Sprintf("Server listening on %s", addr))

	// Start HTTP server in a goroutine.
	go func() {
		if err := http.ListenAndServe(addr, handler); err != nil {
			postLog.Fatal("Server error: " + err.Error())
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	postLog.Info("Shutting down Nukumizu server...")

	// Stop WebSocket client.
	if wsClient != nil {
		wsClient.Stop()
	}

	// Stop all controllers.
	mgr := controller.GetManager()
	if mgr != nil {
		mgr.StopAll()
	}

	postLog.Info("Server stopped")
}

// initControllers initializes and starts all configured controllers.
func initControllers() {
	cfg := config.GetGlobalConfig()
	mgr := controller.GetManager()
	if mgr == nil {
		return
	}

	// QQ (Napcat) controller.
	qqCtrl := qq_napcat.NewQQController(cfg.ControllerMethod.QQ)
	mgr.Register(qqCtrl)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("QQ controller panic: %v", r))
			}
		}()
		if err := qqCtrl.Start(); err != nil {
			postLog.Error("Failed to start QQ controller: " + err.Error())
		}
	}()

	// Telegram controller.
	tgCtrl := telegram.NewTelegramController(cfg.ControllerMethod.Telegram)
	mgr.Register(tgCtrl)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Telegram controller panic: %v", r))
			}
		}()
		if err := tgCtrl.Start(); err != nil {
			postLog.Error("Failed to start Telegram controller: " + err.Error())
		}
	}()

	// Email controller (status-only).
	emailCtrl := pipes.NewEmailController(cfg.ControllerMethod.Email)
	mgr.Register(emailCtrl)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Email controller panic: %v", r))
			}
		}()
		if err := emailCtrl.Start(); err != nil {
			postLog.Error("Failed to start Email controller: " + err.Error())
		}
	}()

	// Ntfy controller (status-only).
	ntfyCtrl := pipes.NewNtfyController(cfg.ControllerMethod.Ntfy)
	mgr.Register(ntfyCtrl)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Ntfy controller panic: %v", r))
			}
		}()
		if err := ntfyCtrl.Start(); err != nil {
			postLog.Error("Failed to start Ntfy controller: " + err.Error())
		}
	}()

	// Webhook controller (status-only).
	webhookCtrl := pipes.NewWebhookController(cfg.ControllerMethod.Webhook)
	mgr.Register(webhookCtrl)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Webhook controller panic: %v", r))
			}
		}()
		if err := webhookCtrl.Start(); err != nil {
			postLog.Error("Failed to start Webhook controller: " + err.Error())
		}
	}()
}

// startBackgroundTasks starts periodic background goroutines.
func startBackgroundTasks(_ *config.Config) {
	// Refresh node list every 5 minutes.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Node refresh panic: %v", r))
			}
		}()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			client := komari.GetClient()
			if client == nil {
				continue
			}
			nodes, err := client.FetchNodes()
			if err != nil {
				postLog.Warning("Failed to refresh node list: " + err.Error())
				continue
			}
			tracker := node.GetTracker()
			if tracker != nil {
				tracker.UpdateNodeList(komari.BuildNodeListData(nodes))
			}
		}
	}()

	// Re-login to Komari every 12 hours.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Komari re-login panic: %v", r))
			}
		}()
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			postLog.Info("Performing scheduled Komari re-login...")
			if err := komari.LoginAndStart(); err != nil {
				postLog.Error("Komari re-login failed: " + err.Error())
			}
		}
	}()

	// Wire node status change notifications to all controllers.
	tracker := node.GetTracker()
	if tracker != nil {
		tracker.OnStatusChange(func(change node.StatusChange) {
			mgr := controller.GetManager()
			if mgr != nil {
				mgr.NotifyStatusChange(change)
			}
		})
	}

	// Safety net for the initial refresh gate: if some node never reports a
	// status, force-complete the initial refresh after a grace period so status
	// notifications cannot stay blocked forever.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Bootstrap timeout panic: %v", r))
			}
		}()
		time.Sleep(2 * time.Minute) // Add 2 minutes grace period to allow nodes to report status.
		if tracker := node.GetTracker(); tracker != nil {
			tracker.CompleteBootstrap()
		}
	}()
}
