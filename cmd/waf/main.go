package main

import (
	"context"
	"embed"
	"encoding/json"
	_ "expvar"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/internal/degradation"
	"github.com/yxorp/internal/metrics"
	"github.com/yxorp/internal/middleware"
	"github.com/yxorp/internal/proxy"
	"github.com/yxorp/internal/rules"
	"github.com/yxorp/internal/server"
	"github.com/yxorp/internal/stats"
	"github.com/yxorp/internal/validation"
	"github.com/yxorp/pkg/logger"
)

//go:embed web
var webAssets embed.FS

func main() {
	// 1. Initialize Logger
	logger.Init()
	logger.Info("Starting Yxorp WAF...")

	// 2. Initialize Prometheus metrics
	metrics.Init()
	logger.Info("Prometheus metrics initialized")

	// 3. Initialize Degradation Manager
	degradationMgr := degradation.NewManager()
	logger.Info("Degradation manager initialized")

	// 4. Load Configuration
	configPath := "configs/rules.yaml"
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Perform comprehensive validation
	if err := validation.ValidateConfig(cfg); err != nil {
		logger.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}
	logger.Info("Configuration validated successfully")

	// Initialize Config Manager
	cfgManager := config.NewManager(cfg)

	// 5. Initialize Load Balancer
	rp, err := proxy.NewLoadBalancer(cfg.Proxy.Targets, cfg.Proxy.MaxRequestSize)
	if err != nil {
		logger.Error("Failed to initialize load balancer", "error", err)
		os.Exit(1)
	}

	// 6. Initialize Security Rules Engine
	ruleEngine, err := rules.NewEngine(cfg.Security.Rules)
	if err != nil {
		logger.Error("Failed to initialize security rules engine", "error", err)
		degradationMgr.SetComponentStatus(degradation.ComponentRuleEngine, degradation.StatusFailed)
		logger.Warn("Continuing with degraded rule engine")
		ruleEngine = nil
	}

	// Thread-safe container for Rules Engine
	var engineMu sync.RWMutex
	currentEngine := ruleEngine

	// Config Watcher
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		var lastMod time.Time
		for range ticker.C {
			info, err := os.Stat(configPath)
			if err != nil {
				continue
			}
			if !lastMod.IsZero() && info.ModTime().After(lastMod) {
				logger.Info("Configuration change detected, reloading...")
				newCfg, err := config.LoadConfig(configPath)
				if err != nil {
					logger.Error("Failed to reload config", "error", err)
					metrics.RecordConfigReloadFailure()
					continue
				}

				// Validate new configuration before applying
				if err := validation.ValidateConfig(newCfg); err != nil {
					logger.Error("Configuration validation failed, keeping old config", "error", err)
					metrics.RecordConfigReloadFailure()
					continue
				}

				newEngine, err := rules.NewEngine(newCfg.Security.Rules)
				if err != nil {
					logger.Error("Failed to reload rules", "error", err)
					metrics.RecordConfigReloadFailure()
					continue
				}

				cfgManager.Set(newCfg)

				engineMu.Lock()
				currentEngine = newEngine
				engineMu.Unlock()

				metrics.RecordConfigReload()
				logger.Info("Configuration reloaded successfully")
			}
			lastMod = info.ModTime()
		}
	}()

	// 7. Initialize Rate Limiter
	rateLimiter := middleware.NewRateLimiter(cfg.Security.RateLimit)

	// 8. Setup Middleware Chain
	// Request Flow: Client -> [Rate Limiter] -> [Security Rules Engine] -> [Request Logger] -> [Circuit Breaker] -> [Reverse Proxy] -> Target Server

	// We build the chain from outer to inner.
	// The handler passed to Chain is the final handler (Reverse Proxy).
	// The middlewares are applied in order.

	// Current available middlewares:
	// - RecoveryMiddleware (Top level)
	// - DegradationMiddleware (passes degradation manager to context)
	// - MetricsMiddleware
	// - RateLimiter
	// - SecurityMiddleware (User-Agent blocking + Rules Engine)
	// - RequestLogger
	// - CircuitBreaker

	finalHandler := middleware.Chain(
		rp,
		middleware.RecoveryMiddleware,
		middleware.DegradationMiddleware(degradationMgr),
		middleware.RequestIDMiddleware(),
		middleware.SecureHeadersMiddleware(),
		middleware.GzipMiddleware(),
		middleware.MetricsMiddleware,
		rateLimiter.Middleware,
		middleware.SecurityMiddleware(
			func() config.SecurityConfig { return cfgManager.Get().Security },
			func() *rules.Engine {
				engineMu.RLock()
				defer engineMu.RUnlock()
				return currentEngine
			},
		),
		middleware.RequestLogger,
	)

	// 9. Start Server
	srv := server.NewServer(cfg.Server, finalHandler)

	// Start Metrics Server (separate port)
	go func() {
		logger.Info("Metrics server listening", "port", "8081")

		// Create API authentication middleware
		apiAuth := middleware.APIAuthMiddleware(cfg.Server.APIKey)

		// Serve embedded dashboard
		webFS, err := fs.Sub(webAssets, "web")
		if err != nil {
			logger.Error("Failed to load web assets", "error", err)
		} else {
			http.Handle("/", http.FileServer(http.FS(webFS)))
		}

		// Prometheus metrics endpoint (public, no auth required)
		http.Handle("/metrics", metrics.Handler())

		// Protected API Endpoints
		http.Handle("/api/logs", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(stats.GetRecentLogs())
		})))

		http.Handle("/api/stats", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(stats.GetSystemStats())
		})))

		http.Handle("/api/rules", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			engineMu.RLock()
			defer engineMu.RUnlock()
			json.NewEncoder(w).Encode(cfgManager.Get().Security.Rules)
		})))

		http.Handle("/api/degradation", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			statuses := degradationMgr.GetAllComponentStatuses()
			mode := degradationMgr.GetMode()

			response := map[string]interface{}{
				"mode":       mode,
				"components": statuses,
			}
			json.NewEncoder(w).Encode(response)
		})))

		http.Handle("/api/config", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(cfgManager.Get())
				return
			}
			if r.Method == http.MethodPost {
				var newCfg config.Config
				if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				// Validate configuration before applying
				if err := validation.ValidateConfig(&newCfg); err != nil {
					http.Error(w, "Configuration validation failed: "+err.Error(), http.StatusBadRequest)
					metrics.RecordConfigReloadFailure()
					return
				}

				if err := cfgManager.Update(configPath, &newCfg); err != nil {
					http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
					return
				}

				// Force reload of components?
				// The main loop watches the file, so it should pick up changes automatically within 10s.
				// But we can also manually trigger updates if we exposed them.
				// Relying on file watcher is safer but slower.
				// Let's explicitly update the in-memory rules engine right away if rules changed.

				// Re-init rules engine for immediate effect
				newEngine, err := rules.NewEngine(newCfg.Security.Rules)
				if err == nil {
					engineMu.Lock()
					currentEngine = newEngine
					engineMu.Unlock()
					metrics.RecordConfigReload()
				} else {
					metrics.RecordConfigReloadFailure()
				}

				json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Configuration saved. Reloading..."})
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))

		// expvar registers handlers on http.DefaultServeMux
		if err := http.ListenAndServe(":8081", nil); err != nil {
			logger.Error("Metrics server failed", "error", err)
		}
	}()

	go func() {
		logger.Info("Server listening", "port", cfg.Server.Port)
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 8. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exited properly")
}
