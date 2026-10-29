package paymentsvc

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
	"github.com/tchoupi663/weave/services/app/internal/ids"
)

// Service is a stateless Payment service that calls the External Payment Provider.
type Service struct {
	externalURL string
	client      *http.Client
}

// New returns a Payment service.
func New(externalURL string, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{}
	}
	return &Service{
		externalURL: strings.TrimRight(externalURL, "/"),
		client:      client,
	}
}

// Handler returns the Payment HTTP handler.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/payments", s.handleCreate)
	mux.HandleFunc("POST /internal/reset", s.handleReset)
	mux.HandleFunc("GET /health", httpx.Health)
	return mux
}

type createRequest struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
	Card    string  `json:"card"`
}

type chargeRequest struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
	Card    string  `json:"card"`
}

type chargeResponse struct {
	ChargeID string `json:"chargeId"`
	Status   string `json:"status"`
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-User") == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing user")
		return
	}
	var req createRequest
	if !httpx.ReadJSON(w, r, &req) {
		return
	}
	if req.OrderID == "" || req.Amount <= 0 || req.Card == "" {
		httpx.Error(w, http.StatusBadRequest, "orderId, a positive amount, and card are required")
		return
	}

	body, err := json.Marshal(chargeRequest{OrderID: req.OrderID, Amount: req.Amount, Card: req.Card})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not encode charge")
		return
	}
	chargeReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.externalURL+"/charge", bytes.NewReader(body))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not build charge")
		return
	}
	chargeReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(chargeReq)
	if err != nil {
		slog.Error("charge external", "err", err)
		httpx.Error(w, http.StatusBadGateway, "external payment provider unavailable")
		return
	}
	defer resp.Body.Close() //nolint:errcheck
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "could not read external response")
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("external charge failed", "status", resp.StatusCode, "body", string(respBody))
		httpx.Error(w, http.StatusBadGateway, "external payment provider failed")
		return
	}
	var charge chargeResponse
	if err := json.Unmarshal(respBody, &charge); err != nil || charge.Status != "succeeded" {
		slog.Error("invalid external charge response", "err", err)
		httpx.Error(w, http.StatusBadGateway, "invalid external payment response")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"paymentId": ids.New(),
		"status":    "succeeded",
	})
}

func (s *Service) handleReset(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
