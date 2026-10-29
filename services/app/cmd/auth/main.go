package main

import (
	"log/slog"
	"os"

	"github.com/tchoupi663/weave/services/app/internal/authsvc"
	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

func main() {
	ctx, stop := httpx.SignalContext()
	defer stop()

	port := httpx.Env("PORT", "8081")
	if err := httpx.Run(ctx, ":"+port, authsvc.New().Handler(), nil); err != nil {
		slog.Error("auth stopped", "err", err)
		os.Exit(1)
	}
}
