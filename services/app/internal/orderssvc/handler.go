package orderssvc

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
	"github.com/tchoupi663/weave/services/app/internal/ids"
)

// Handler serves the Orders HTTP API.
type Handler struct {
	store Store
}

// NewHandler returns an Orders handler.
func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

// Routes returns the Orders HTTP handler.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/orders", h.handleCreate)
	mux.HandleFunc("GET /api/orders/{id}", h.handleGet)
	mux.HandleFunc("POST /internal/reset", h.handleReset)
	mux.HandleFunc("GET /health", h.handleHealth)
	return mux
}

type createRequest struct {
	Items []Item  `json:"items"`
	Total float64 `json:"total"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User")
	if userID == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing user")
		return
	}
	var req createRequest
	if !httpx.ReadJSON(w, r, &req) {
		return
	}
	if len(req.Items) == 0 || req.Total <= 0 {
		httpx.Error(w, http.StatusBadRequest, "items and a positive total are required")
		return
	}
	for _, item := range req.Items {
		if item.ProductID == "" || item.Quantity <= 0 {
			httpx.Error(w, http.StatusBadRequest, "each item needs a productId and a positive quantity")
			return
		}
	}

	order := Order{
		ID:        ids.New(),
		UserID:    userID,
		Status:    "created",
		Total:     req.Total,
		Items:     req.Items,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.Create(r.Context(), order); err != nil {
		slog.Error("create order", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "could not create order")
		return
	}
	httpx.JSON(w, http.StatusCreated, order)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User")
	if userID == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing user")
		return
	}
	order, err := h.store.Get(r.Context(), r.PathValue("id"), userID)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("get order", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "could not read order")
		return
	}
	httpx.JSON(w, http.StatusOK, order)
}

func (h *Handler) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Reset(r.Context()); err != nil {
		slog.Error("reset orders", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "could not reset orders")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.Health(w, r)
}
