package main

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
	"github.com/tchoupi663/weave/services/app/internal/paymentsvc"
)

func main() {
	ctx, stop := httpx.SignalContext()
	defer stop()

	externalURL := httpx.Env("EXTERNAL_URL", "https://localhost:8443")
	insecure := httpx.EnvBool("EXTERNAL_INSECURE", true)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // dummy external provider
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}

	port := httpx.Env("PORT", "8084")
	if err := httpx.Run(ctx, ":"+port, paymentsvc.New(externalURL, client).Handler(), nil); err != nil {
		slog.Error("payment stopped", "err", err)
		os.Exit(1)
	}
}
