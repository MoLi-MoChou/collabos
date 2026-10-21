// Command server runs the CollabOS API, the UI realtime hub and the daemon hub.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/collabos/collabos/server/internal/config"
	"github.com/collabos/collabos/server/internal/handler"
	"github.com/collabos/collabos/server/internal/logger"
	"github.com/collabos/collabos/server/internal/service"
	"github.com/collabos/collabos/server/migrations"
)

func main() {
	log := logger.New("server")
	cfg, err := config.LoadServer()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("COLLAB_AUTO_MIGRATE") == "true" {
		if err := migrations.Up(cfg.DatabaseURL); err != nil {
			log.Error("migrate", "err", err)
			os.Exit(1)
		}
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Error("db ping", "err", err)
		os.Exit(1)
	}
	svc := service.New(pool, cfg)
	h := handler.New(svc, os.Getenv("COLLAB_WEB_DIR"))
	go svc.RunSweeper(ctx)
	srv := &http.Server{Addr: cfg.Addr, Handler: h.Router(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Info("listening", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
