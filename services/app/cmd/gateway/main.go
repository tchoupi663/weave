package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/tchoupi663/weave/services/app/internal/gatewaysvc"
	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

func main() {
	ctx, stop := httpx.SignalContext()
	defer stop()

	cfg := gatewaysvc.Config{
		AuthURL:    httpx.Env("AUTH_URL", "http://localhost:8081"),
		CatalogURL: httpx.Env("CATALOG_URL", "http://localhost:8082"),
		OrdersURL:  httpx.Env("ORDERS_URL", "http://localhost:8083"),
		PaymentURL: httpx.Env("PAYMENT_URL", "http://localhost:8084"),
	}
	client := &http.Client{Timeout: 5 * time.Second}

	port := httpx.Env("PORT", "8080")
	if err := httpx.Run(ctx, ":"+port, gatewaysvc.New(cfg, client).Handler(), nil); err != nil {
		slog.Error("gateway stopped", "err", err)
		os.Exit(1)
	}
}
