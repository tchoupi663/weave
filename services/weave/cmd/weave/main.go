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

	"github.com/tchoupi663/weave/services/weave/internal/api"
	"github.com/tchoupi663/weave/services/weave/internal/config"
	"github.com/tchoupi663/weave/services/weave/internal/hubble"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("weave stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store := topology.NewStore(cfg.WatchNamespace, cfg.Window)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := hubble.New(cfg.HubbleRelayAddr, store, log)
	go client.Run(ctx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(store, cfg.WebDir, log),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("weave listening",
			"addr", cfg.ListenAddr,
			"namespace", cfg.WatchNamespace,
			"window", cfg.Window.String(),
			"web_dir", cfg.WebDir,
		)
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
