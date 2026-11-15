package history

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/httpx"
)

type edgeMetricRow struct {
	BucketStart   time.Time `json:"bucket_start"`
	EdgeID        string    `json:"edge_id"`
	Source        string    `json:"source"`
	Target        string    `json:"target"`
	Protocol      string    `json:"protocol"`
	Port          uint32    `json:"port"`
	Connections   uint64    `json:"connections"`
	Events        uint64    `json:"events"`
	Requests      uint64    `json:"requests"`
	Responses     uint64    `json:"responses"`
	Status2xx     uint64    `json:"status_2xx"`
	Status3xx     uint64    `json:"status_3xx"`
	Status4xx     uint64    `json:"status_4xx"`
	Status5xx     uint64    `json:"status_5xx"`
	StatusOther   uint64    `json:"status_other"`
	P50MS         float64   `json:"p50_ms"`
	P95MS         float64   `json:"p95_ms"`
	LatencySource string    `json:"latency_source"`
}

type routeMetricRow struct {
	BucketStart time.Time `json:"bucket_start"`
	EdgeID      string    `json:"edge_id"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Requests    uint64    `json:"requests"`
	Responses   uint64    `json:"responses"`
	Status2xx   uint64    `json:"status_2xx"`
	Status3xx   uint64    `json:"status_3xx"`
	Status4xx   uint64    `json:"status_4xx"`
	Status5xx   uint64    `json:"status_5xx"`
	StatusOther uint64    `json:"status_other"`
}

func (s *Server) metricsEdges(w http.ResponseWriter, r *http.Request) {
	from, to, bucket, err := parseMetricRange(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	table := "edge_metrics_5s"
	bucketSeconds := 5
	if bucket == "1m" {
		table = "edge_metrics_1m"
		bucketSeconds = 60
	}
	args := []any{from.UTC(), to.UTC()}
	filter := ""
	if edgeIDs := r.URL.Query()["edge_id"]; len(edgeIDs) > 0 {
		placeholders := make([]string, 0, len(edgeIDs))
		for _, edgeID := range edgeIDs {
			if strings.TrimSpace(edgeID) == "" {
				httpx.WriteError(w, http.StatusBadRequest, "edge_id must not be empty")
				return
			}
			placeholders = append(placeholders, "?")
			args = append(args, edgeID)
		}
		filter = " AND edge_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	query := fmt.Sprintf(`SELECT bucket_start, edge_id, source, target, protocol, port,
       connections, events, requests, responses, status_2xx, status_3xx, status_4xx, status_5xx, status_other,
       p50_ms, p95_ms, latency_source
FROM %s
WHERE bucket_start >= ? AND bucket_start < ?%s
ORDER BY bucket_start, edge_id`, table, filter)
	rows, err := s.hist.duck.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "metric store unavailable")
		return
	}
	defer rows.Close()
	result := make([]edgeMetricRow, 0)
	for rows.Next() {
		var row edgeMetricRow
		if err := rows.Scan(&row.BucketStart, &row.EdgeID, &row.Source, &row.Target, &row.Protocol, &row.Port,
			&row.Connections, &row.Events, &row.Requests, &row.Responses, &row.Status2xx, &row.Status3xx, &row.Status4xx, &row.Status5xx, &row.StatusOther,
			&row.P50MS, &row.P95MS, &row.LatencySource); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		row.BucketStart = row.BucketStart.UTC()
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"bucket_seconds": bucketSeconds,
		"from":           from.UTC(),
		"to":             to.UTC(),
		"rows":           result,
	})
}

func (s *Server) metricsRoutes(w http.ResponseWriter, r *http.Request) {
	from, to, bucket, err := parseMetricRange(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if bucket != "5s" {
		httpx.WriteError(w, http.StatusBadRequest, "route metrics only support bucket=5s")
		return
	}
	args := []any{from.UTC(), to.UTC()}
	filters := []string{}
	add := func(column string, values []string) error {
		if len(values) == 0 {
			return nil
		}
		placeholders := make([]string, 0, len(values))
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s must not be empty", column)
			}
			placeholders = append(placeholders, "?")
			args = append(args, value)
		}
		filters = append(filters, column+" IN ("+strings.Join(placeholders, ",")+")")
		return nil
	}
	if err := add("edge_id", r.URL.Query()["edge_id"]); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := add("method", r.URL.Query()["method"]); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := add("path", r.URL.Query()["path"]); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter := ""
	if len(filters) > 0 {
		filter = " AND " + strings.Join(filters, " AND ")
	}
	query := `SELECT bucket_start, edge_id, method, path, requests, responses, status_2xx, status_3xx, status_4xx, status_5xx, status_other
FROM route_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ?` + filter + `
ORDER BY bucket_start, edge_id, method, path`
	rows, err := s.hist.duck.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "metric store unavailable")
		return
	}
	defer rows.Close()
	result := make([]routeMetricRow, 0)
	for rows.Next() {
		var row routeMetricRow
		if err := rows.Scan(&row.BucketStart, &row.EdgeID, &row.Method, &row.Path, &row.Requests, &row.Responses,
			&row.Status2xx, &row.Status3xx, &row.Status4xx, &row.Status5xx, &row.StatusOther); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		row.BucketStart = row.BucketStart.UTC()
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"bucket_seconds": 5,
		"from":           from.UTC(),
		"to":             to.UTC(),
		"rows":           result,
	})
}

func (s *Server) incidentAlerts(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if _, err := s.hist.GetIncident(r.Context(), incidentID); err != nil {
		if err == ErrIncidentNotFound {
			httpx.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	alerts, err := s.hist.AlertsForIncident(r.Context(), incidentID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"alerts": alerts})
}

func (s *Server) recordingGaps(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := parseMetricRange(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	gaps, err := s.hist.Gaps(r.Context(), from, to)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"gaps": gaps})
}

func parseMetricRange(r *http.Request) (time.Time, time.Time, string, error) {
	query := r.URL.Query()
	from, err := time.Parse(time.RFC3339, query.Get("from"))
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("from must be RFC3339")
	}
	to, err := time.Parse(time.RFC3339, query.Get("to"))
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("to must be RFC3339")
	}
	from, to = from.UTC(), to.UTC()
	if !to.After(from) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("to must be after from")
	}
	bucket := query.Get("bucket")
	if bucket == "" {
		bucket = "5s"
	}
	if bucket != "5s" && bucket != "1m" {
		return time.Time{}, time.Time{}, "", fmt.Errorf("bucket must be 5s or 1m")
	}
	return from, to, bucket, nil
}
