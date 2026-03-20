package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/anti-ai-proxy/proxy/internal/anticheat"
	"github.com/anti-ai-proxy/proxy/internal/api"
	"github.com/anti-ai-proxy/proxy/internal/auth"
	"github.com/anti-ai-proxy/proxy/internal/cache"
	"github.com/anti-ai-proxy/proxy/internal/config"
	"github.com/anti-ai-proxy/proxy/internal/database"
	"github.com/anti-ai-proxy/proxy/internal/filter"
	"github.com/anti-ai-proxy/proxy/internal/logging"
	"github.com/anti-ai-proxy/proxy/internal/middleware"
	"github.com/anti-ai-proxy/proxy/internal/proxy"
	"github.com/anti-ai-proxy/proxy/internal/session"
)

func main() {
	// ── Load Config ──
	cfg := config.Load()

	// ── Database ──
	db, err := database.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations("./migrations"); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// ── Redis ──
	rdb, err := cache.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer rdb.Close()

	// ── Initialize Components ──
	filterEngine := filter.New()
	logger := logging.New(db.Pool, cfg.LogLevel)
	authHandler := auth.New(cfg, db.Pool)
	sessionMgr := session.NewManager(db.Pool, rdb.Client, cfg.SessionTimeout)
	scorer := anticheat.NewScorer(db.Pool, rdb.Client, cfg)
	rateLimiter := middleware.NewRateLimiter(rdb.Client, cfg.RateLimitRequests, cfg.RateLimitWindow)

	// ── Proxy Server ──
	proxyServer := proxy.New(filterEngine, sessionMgr, logger, scorer)

	// ── API Server ──
	apiSecret := os.Getenv("PROXY_API_SECRET")
	if apiSecret == "" {
		apiSecret = "shared-secret-change-me"
	}
	apiServer := api.NewServer(db.Pool, authHandler, sessionMgr, filterEngine, scorer, proxyServer, apiSecret)
	apiRouter := apiServer.Router(rateLimiter)

	// ── Start Servers ──
	proxyAddr := fmt.Sprintf("%s:%s", cfg.ProxyHost, cfg.ProxyPort)
	apiAddr := fmt.Sprintf("%s:%s", cfg.ProxyHost, cfg.APIPort)

	// Start proxy in background
	go func() {
		log.Info().Str("addr", proxyAddr).Msg("Starting proxy server")
		if err := proxyServer.ListenAndServe(proxyAddr); err != nil {
			log.Fatal().Err(err).Msg("Proxy server failed")
		}
	}()

	// Start API in background
	go func() {
		log.Info().Str("addr", apiAddr).Msg("Starting API server")
		apiHTTP := &http.Server{
			Addr:    apiAddr,
			Handler: apiRouter,
		}
		if err := apiHTTP.ListenAndServe(); err != nil {
			log.Fatal().Err(err).Msg("API server failed")
		}
	}()

	log.Info().
		Str("proxy", proxyAddr).
		Str("api", apiAddr).
		Int("blocked_domains", filterEngine.Stats()["blocked_domains"]).
		Msg("CTF Anti-AI Proxy Gateway started")

	// ── Graceful Shutdown ──
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down...")
}
