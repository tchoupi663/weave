package history

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/tchoupi663/weave/services/weave/internal/contracts"
	"github.com/tchoupi663/weave/services/weave/internal/httpx"
)

// Server exposes the history service's external incident routes and the
// internal observation receiver. It no longer serves changes; those belong to
// weave-inventory and are proxied by weave-api.
type Server struct {
	hist *History
	log  *slog.Logger
}

// New builds the weave-history HTTP handler.
func New(hist *History, log *slog.Logger) http.Handler {
	s := &Server{hist: hist, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/incidents", s.createIncident)
	mux.HandleFunc("GET /api/incidents", s.listIncidents)
	mux.HandleFunc("GET /api/incidents/{id}", s.getIncident)
	mux.HandleFunc("PATCH /api/incidents/{id}", s.patchIncident)
	mux.HandleFunc("POST /api/incidents/{id}/recompute", s.recomputeIncident)
	mux.HandleFunc("GET /api/incidents/{id}/timeline", s.incidentTimeline)
	mux.HandleFunc("GET /api/incidents/{id}/alerts", s.incidentAlerts)
	mux.HandleFunc("GET /api/metrics/edges", s.metricsEdges)
	mux.HandleFunc("GET /api/metrics/routes", s.metricsRoutes)
	mux.HandleFunc("GET /api/gaps", s.recordingGaps)
	mux.HandleFunc("POST /internal/v1/observations", s.observations)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/health", s.health)
	return httpx.SecurityHeaders(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	status := s.hist.DetectionStatus()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":                    "ok",
		"late_observations":         s.hist.LateObservations(),
		"detection_enabled":         status.Enabled,
		"rules_loaded":              status.RulesLoaded,
		"rule_pack_version":         status.RulePackVersion,
		"alerts_created":            status.AlertsCreated,
		"incidents_created":         status.IncidentsCreated,
		"detection_suppressed_gaps": status.SuppressedGaps,
		"detection_evaluations":     status.Evaluations,
		"last_detection_evaluation": status.LastEvaluation,
	})
}

func (s *Server) observations(w http.ResponseWriter, r *http.Request) {
	var batch contracts.ObservationBatch
	if err := httpx.DecodeJSON(w, r, &batch, contracts.MaxBatchBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "observation batch too large")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(batch.L4)+len(batch.L7) > contracts.MaxObservationsPerBatch {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "observation batch exceeds limit")
		return
	}
	applied, err := s.hist.RecordBatch(r.Context(), batch)
	if err != nil {
		s.log.Error("apply observation batch", "error", err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "history store unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"applied": applied})
}

func (s *Server) createIncident(w http.ResponseWriter, r *http.Request) {
	var request CreateIncidentRequest
	if err := httpx.DecodeJSON(w, r, &request, 1<<20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Start.IsZero() {
		httpx.WriteError(w, http.StatusBadRequest, "start is required")
		return
	}
	incident, err := s.hist.CreateIncident(r.Context(), request)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, incident)
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	incidents, err := s.hist.ListIncidents(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if incidents == nil {
		incidents = []Incident{}
	}
	httpx.WriteJSON(w, http.StatusOK, incidents)
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	incident, err := s.hist.GetIncident(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrIncidentNotFound) {
			httpx.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, incident)
}

func (s *Server) patchIncident(w http.ResponseWriter, r *http.Request) {
	var patch PatchIncidentRequest
	if err := httpx.DecodeJSON(w, r, &patch, 1<<20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	incident, err := s.hist.UpdateIncident(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		if errors.Is(err, ErrIncidentNotFound) {
			httpx.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, incident)
}

func (s *Server) recomputeIncident(w http.ResponseWriter, r *http.Request) {
	incident, err := s.hist.RecomputeIncident(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrIncidentNotFound) {
			httpx.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, incident)
}

func (s *Server) incidentTimeline(w http.ResponseWriter, r *http.Request) {
	timeline, err := s.hist.IncidentTimeline(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrIncidentNotFound) {
			httpx.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, timeline)
}
