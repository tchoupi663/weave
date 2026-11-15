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
	"github.com/tchoupi663/weave/services/weave/internal/history"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("weave-history stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadHistory()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	hist, err := history.Open(cfg.DataDir, cfg.WatchNamespace, cfg.FlushGrace, log)
	if err != nil {
		return err
	}
	defer hist.Close()
	if err := hist.SetBaseline(cfg.Baseline); err != nil {
		return err
	}
	if err := hist.ConfigureDetection(cfg.SLAFile, cfg.DetectionEvery, cfg.GroupingWindow, cfg.Cooldown); err != nil {
		log.Error("SLA detection disabled", "error", err)
	} else if cfg.SLAFile != "" {
		log.Info("SLA detection configured", "file", cfg.SLAFile, "baseline", cfg.Baseline.String())
	}
	hist.Start(ctx)
	metrics5, metrics1, err := hist.Counts(ctx)
	if err != nil {
		log.Warn("history counts unavailable", "error", err)
	} else {
		log.Info("history ready", "data_dir", cfg.DataDir, "edge_5s", metrics5, "edge_1m", metrics1)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           history.New(hist, log),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("weave-history listening", "addr", cfg.ListenAddr, "namespace", cfg.WatchNamespace,
			"flush_grace", cfg.FlushGrace.String())
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
