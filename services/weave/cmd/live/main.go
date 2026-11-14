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
	"github.com/tchoupi663/weave/services/weave/internal/live"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("weave-live stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadLive()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	service := live.New(live.Config{
		ListenAddr:      cfg.ListenAddr,
		HubbleRelayAddr: cfg.HubbleRelayAddr,
		WatchNamespace:  cfg.WatchNamespace,
		Window:          cfg.Window,
		HistoryURL:      cfg.HistoryURL,
		FeedInterval:    cfg.FeedInterval,
		FeedBatch:       cfg.FeedBatch,
	}, log)
	service.Start(ctx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("weave-live listening", "addr", cfg.ListenAddr, "namespace", cfg.WatchNamespace,
			"window", cfg.Window.String(), "history_url", cfg.HistoryURL)
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
	service.Stop()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
