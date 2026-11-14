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
	"github.com/tchoupi663/weave/services/weave/internal/inventory"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("weave-inventory stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadInventory()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	inv, err := inventory.New(inventory.Config{
		ListenAddr:        cfg.ListenAddr,
		WatchNamespace:    cfg.WatchNamespace,
		DataDir:           cfg.DataDir,
		LiveURL:           cfg.LiveURL,
		LiveTimeout:       cfg.LiveTimeout,
		ReconcileInterval: cfg.ReconcileInterval,
		ArgoCDURL:         cfg.ArgoCDURL,
		ArgoCDToken:       cfg.ArgoCDToken,
		ArgoCDApp:         cfg.ArgoCDApp,
		ArgoCDNamespace:   cfg.ArgoCDNamespace,
	}, log)
	if err != nil {
		return err
	}
	defer inv.Close()
	inv.Start(ctx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           inv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("weave-inventory listening", "addr", cfg.ListenAddr, "namespace", cfg.WatchNamespace,
			"live_url", cfg.LiveURL, "argo", cfg.ArgoCDURL != "")
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
