package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/config"
	"github.com/tchoupi663/weave/services/weave/internal/edge"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("weave-api stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadEdge()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	service, err := edge.New(edge.Config{
		ListenAddr:    cfg.ListenAddr,
		WebDir:        cfg.WebDir,
		LiveURL:       cfg.LiveURL,
		HistoryURL:    cfg.HistoryURL,
		InventoryURL:  cfg.InventoryURL,
		HealthTimeout: cfg.HealthTimeout,
	}, log)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("weave-api listening", "addr", cfg.ListenAddr, "web_dir", cfg.WebDir,
			"live_url", cfg.LiveURL, "history_url", cfg.HistoryURL, "inventory_url", cfg.InventoryURL)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
