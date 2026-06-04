package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/api"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/storage"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/user"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
	redisclient "github.com/omnichannel/omni-chan-v2/pkg/redis"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.App.Env)

	if err := migrate.Up(cfg.Database.URL, "migrations"); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}

	db, err := database.New(ctx, cfg.Database)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	rdb, err := redisclient.New(ctx, cfg.Redis)
	if err != nil {
		log.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	store, err := storage.New(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	_ = store

	wsHub := hub.New(log, rdb.Client)
	go func() {
		if err := wsHub.StartRedisSubscriber(ctx); err != nil && ctx.Err() == nil {
			log.Error("websocket redis subscriber stopped", "error", err)
		}
	}()

	jwt := jwtmanager.NewManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessTTL,
	)

	userRepo := user.NewRepository(db)
	teamRepo := team.NewRepository(db)
	refreshRepo := auth.NewRefreshRepository(db)
	authService := auth.NewService(userRepo, teamRepo, refreshRepo, jwt, cfg.JWT)
	authHandler := auth.NewHandler(authService)

	teamService := team.NewService(teamRepo, userRepo)
	teamHandler := team.NewHandler(teamService)

	app := api.NewRouter(api.Deps{
		Config:      cfg,
		Logger:      log,
		DB:          db,
		AuthHandler: authHandler,
		TeamHandler: teamHandler,
		JWT:         jwt,
	})

	go func() {
		log.Info("server starting", "addr", cfg.HTTP.Addr())
		if err := app.Listen(cfg.HTTP.Addr()); err != nil {
			log.Error("server stopped", "error", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = shutdownCtx
	_ = app.Shutdown()
}
