package main

import (
	"log/slog"
	"os"

	"github.com/tchoupi663/weave/services/app/internal/catalogsvc"
	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

func main() {
	ctx, stop := httpx.SignalContext()
	defer stop()

	port := httpx.Env("PORT", "8082")
	if err := httpx.Run(ctx, ":"+port, catalogsvc.New().Handler(), nil); err != nil {
		slog.Error("catalog stopped", "err", err)
		os.Exit(1)
	}
}
