package live

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/contracts"
	"github.com/tchoupi663/weave/services/weave/internal/httpx"
)

type httpHandler struct {
	live *Live
	log  *slog.Logger
}

func newHTTPHandler(l *Live) http.Handler {
	h := &httpHandler{live: l, log: l.log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/topology", h.topology)
	mux.HandleFunc("GET /api/stream", h.stream)
	mux.HandleFunc("POST /api/reset", h.reset)
	mux.HandleFunc("POST /v1/traces", h.traces)
	mux.HandleFunc("GET /internal/v1/workloads", h.workloads)
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /api/health", h.health)
	return httpx.SecurityHeaders(mux)
}

func (h *httpHandler) topology(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, h.live.store.Snapshot())
}

func (h *httpHandler) reset(w http.ResponseWriter, _ *http.Request) {
	h.live.store.Reset()
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (h *httpHandler) health(w http.ResponseWriter, _ *http.Request) {
	snapshot := h.live.store.Snapshot()
	status := "ok"
	if !snapshot.Hubble.Connected {
		status = "degraded"
	}
	feed := h.live.FeedStatus()
	body := map[string]any{
		"status":                status,
		"hubble_connected":      snapshot.Hubble.Connected,
		"observed_flows":        snapshot.Hubble.Observed,
		"history_connected":     feed.HistoryConnected,
		"recording_gaps":        feed.RecordingGaps,
		"dropped_observations":  feed.DroppedObservations,
		"gap_active":            feed.GapActive,
		"buffered_observations": feed.Buffered,
	}
	if feed.LastError != "" {
		body["last_delivery_error"] = feed.LastError
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *httpHandler) workloads(w http.ResponseWriter, _ *http.Request) {
	observed := h.live.store.ObservedWorkloads()
	response := contracts.WorkloadsResponse{Workloads: make([]contracts.Workload, 0, len(observed))}
	for _, workload := range observed {
		response.Workloads = append(response.Workloads, contracts.Workload{
			Workload:  workload.Workload,
			Kind:      workload.Kind,
			FirstSeen: workload.FirstSeen.UTC(),
			LastSeen:  workload.LastSeen.UTC(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *httpHandler) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 1500\n\n")
	flusher.Flush()

	send := func() bool {
		payload, err := json.Marshal(h.live.store.Snapshot())
		if err != nil {
			h.log.Error("marshal topology snapshot", "error", err)
			return false
		}
		if _, err := fmt.Fprintf(w, "event: topology\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send() {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}
