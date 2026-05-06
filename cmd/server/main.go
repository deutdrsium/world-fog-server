package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/xuefz/world-fog/internal/config"
	"github.com/xuefz/world-fog/internal/db"
	"github.com/xuefz/world-fog/internal/handler"
	mw "github.com/xuefz/world-fog/internal/middleware"
	"github.com/xuefz/world-fog/internal/store"
	"github.com/xuefz/world-fog/internal/token"
	webauthnutil "github.com/xuefz/world-fog/internal/webauthn"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if cfg.JWT.Secret == "" || cfg.JWT.Secret == "REPLACE_WITH_32_BYTE_RANDOM_SECRET" {
		slog.Error("WF_JWT_SECRET must be set to a secure random value")
		os.Exit(1)
	}

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	userStore := store.NewUserStore(database)
	credStore := store.NewCredentialStore(database)
	sessStore := store.NewSessionStore(database)
	fogTileStore := store.NewFogTileStore(database)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sessStore.StartCleanup(ctx, 10*time.Minute)

	wa, err := webauthnutil.New(&cfg.WebAuthn)
	if err != nil {
		slog.Error("init webauthn", "err", err)
		os.Exit(1)
	}

	tm := token.NewManager(cfg.JWT.Secret, cfg.JWT.ExpiryHrs)

	authH := handler.NewAuthHandler(wa, database, userStore, credStore, sessStore, tm)
	meH := handler.NewMeHandler(userStore)
	wkH := handler.NewWellKnownHandler(&cfg.Apple)
	fogH := handler.NewFogHandler(fogTileStore)

	r := buildRouter(cfg, authH, meH, wkH, fogH, tm)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		slog.Info("shutting down")
		cancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}()

	slog.Info("server starting", "addr", addr, "tls", cfg.Server.TLSCert != "")

	if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
		err = srv.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey)
	} else {
		slog.Warn("TLS not configured, running HTTP — not suitable for production")
		err = srv.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func buildRouter(
	cfg *config.Config,
	authH *handler.AuthHandler,
	meH *handler.MeHandler,
	wkH *handler.WellKnownHandler,
	fogH *handler.FogHandler,
	tm *token.Manager,
) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(mw.CORS(cfg.WebAuthn.RPOrigins))

	// Well-known endpoints (no auth required, served on all origins).
	r.Get("/.well-known/apple-app-site-association", wkH.AppleAppSiteAssociation)
	r.Get("/.well-known/webauthn", wkH.WebAuthnRelatedOrigins)

	// Public auth endpoints.
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register/begin", authH.RegisterBegin)
		r.Post("/register/finish", authH.RegisterFinish)
		r.Post("/login/begin", authH.LoginBegin)
		r.Post("/login/finish", authH.LoginFinish)
	})

	// Protected endpoints.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.Authenticate(tm))
		r.Get("/me", meH.GetMe)
		r.Get("/fog/tiles", fogH.ListTiles)
		r.Get("/fog/tiles/{z}/{x}/{y}", fogH.GetTile)
		r.Put("/fog/tiles/{z}/{x}/{y}", fogH.PutTile)
	})

	return r
}
