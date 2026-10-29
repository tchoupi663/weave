package main

import (
	"log/slog"
	"os"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
	"github.com/tchoupi663/weave/services/app/internal/orderssvc"
)

func main() {
	ctx, stop := httpx.SignalContext()
	defer stop()

	dsn := httpx.Env("DATABASE_URL", "postgres://weave:weave@localhost:5432/weave?sslmode=disable")
	store, err := orderssvc.OpenPostgres(ctx, dsn)
	if err != nil {
		slog.Error("open postgres", "err", err)
		os.Exit(1)
	}
	defer store.Close() //nolint:errcheck

	port := httpx.Env("PORT", "8083")
	if err := httpx.Run(ctx, ":"+port, orderssvc.NewHandler(store).Routes(), nil); err != nil {
		slog.Error("orders stopped", "err", err)
		os.Exit(1)
	}
}
