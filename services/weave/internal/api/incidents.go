package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/history"
)

func (s *Server) createIncident(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	var request history.CreateIncidentRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Start.IsZero() {
		writeError(w, http.StatusBadRequest, "start is required")
		return
	}
	incident, err := s.history.CreateIncident(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, incident)
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	incidents, err := s.history.ListIncidents(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if incidents == nil {
		incidents = []history.Incident{}
	}
	writeJSON(w, http.StatusOK, incidents)
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	incident, err := s.history.GetIncident(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, history.ErrIncidentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (s *Server) patchIncident(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	var patch history.PatchIncidentRequest
	if err := decodeJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	incident, err := s.history.UpdateIncident(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		if errors.Is(err, history.ErrIncidentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (s *Server) recomputeIncident(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	incident, err := s.history.RecomputeIncident(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, history.ErrIncidentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (s *Server) incidentTimeline(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	timeline, err := s.history.IncidentTimeline(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, history.ErrIncidentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusServiceUnavailable, "history is not enabled")
		return
	}
	to := time.Now().UTC()
	from := to.Add(-24 * time.Hour)
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
		from = parsed.UTC()
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
		to = parsed.UTC()
	}
	limit := 1000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	events, err := s.history.Changes(r.Context(), from, to, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []history.ChangeEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
