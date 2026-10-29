package main

import (
	"log/slog"
	"os"

	"github.com/tchoupi663/weave/services/app/internal/externalsvc"
	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

func main() {
	ctx, stop := httpx.SignalContext()
	defer stop()

	tlsConfig, err := externalsvc.SelfSignedTLSConfig("external", "localhost")
	if err != nil {
		slog.Error("build external TLS config", "err", err)
		os.Exit(1)
	}

	port := httpx.Env("PORT", "8443")
	if err := httpx.Run(ctx, ":"+port, externalsvc.New().Handler(), tlsConfig); err != nil {
		slog.Error("external stopped", "err", err)
		os.Exit(1)
	}
}
