package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/api"
	"github.com/tchoupi663/weave/services/weave/internal/config"
	"github.com/tchoupi663/weave/services/weave/internal/history"
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

	var hist *history.History
	if cfg.DataDir != "" {
		var argo *history.ArgoClient
		if cfg.ArgoCDURL != "" {
			if cfg.ArgoCDToken == "" {
				return fmt.Errorf("ARGOCD_TOKEN is required when ARGOCD_URL is set")
			}
			argo = history.NewArgoClient(cfg.ArgoCDURL, cfg.ArgoCDToken, cfg.ArgoCDApp, cfg.ArgoCDNamespace)
		}
		hist, err = history.Open(cfg.DataDir, cfg.WatchNamespace, store, argo, log)
		if err != nil {
			return fmt.Errorf("open history: %w", err)
		}
		store.SetRecorder(hist)
		hist.Start(ctx)
		defer hist.Close()
		metrics5, metrics1, changes, countErr := hist.Counts(ctx)
		if countErr == nil {
			log.Info("history ready", "data_dir", cfg.DataDir, "edge_5s", metrics5, "edge_1m", metrics1, "changes", changes)
		}
	}

	client := hubble.New(cfg.HubbleRelayAddr, store, log)
	go client.Run(ctx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewWithHistory(store, cfg.WebDir, hist, log),
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
