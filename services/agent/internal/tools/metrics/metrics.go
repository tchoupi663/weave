// Package metrics exposes production-shaped metric queries over Weave history.
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

type Config struct {
	WeaveURL string
	Guard    *scope.Guard
}

type Client struct {
	baseURL string
	guard   *scope.Guard
	http    *http.Client
}

func New(cfg Config) *Client {
	return &Client{baseURL: strings.TrimRight(cfg.WeaveURL, "/"), guard: cfg.Guard, http: &http.Client{Timeout: 15 * time.Second}}
}

const (
	MetricEdgeRequests  = "weave_edge_requests"
	MetricEdgeErrors    = "weave_edge_errors"
	MetricEdgeP95       = "weave_edge_p95_ms"
	MetricRouteRequests = "weave_route_requests"
	MetricRouteErrors   = "weave_route_errors"
)

type metricInfo struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

var metricCatalog = []metricInfo{
	{Name: MetricEdgeRequests, Kind: "edge", Description: "Observed HTTP requests on an edge per 5s bucket", Labels: []string{"edge_id", "source", "target"}},
	{Name: MetricEdgeErrors, Kind: "edge", Description: "Observed 4xx plus 5xx responses on an edge per 5s bucket", Labels: []string{"edge_id", "source", "target"}},
	{Name: MetricEdgeP95, Kind: "edge", Description: "Observed p95 latency in milliseconds on an edge", Labels: []string{"edge_id", "source", "target", "latency_source"}},
	{Name: MetricRouteRequests, Kind: "route", Description: "Observed requests on a normalized method/path route", Labels: []string{"edge_id", "method", "path"}},
	{Name: MetricRouteErrors, Kind: "route", Description: "Observed 4xx plus 5xx responses on a normalized method/path route", Labels: []string{"edge_id", "method", "path"}},
}

func Register(server *mcp.Server, cfg Config) {
	client := New(cfg)
	mcp.AddTool(server, &mcp.Tool{Name: "list_metrics", Description: "List the metric series available in Weave history."}, client.listMetrics)
	mcp.AddTool(server, &mcp.Tool{Name: "query_metric", Description: "Query one metric over a time range. Results are filtered to the investigation scope in a confined run."}, client.queryMetric)
	mcp.AddTool(server, &mcp.Tool{Name: "query_metric_range", Description: "Query one metric over a time range with optional downsampling."}, client.queryMetricRange)
	mcp.AddTool(server, &mcp.Tool{Name: "list_metric_labels", Description: "List the labels for a metric."}, client.listMetricLabels)
}

type listMetricsOut struct {
	Metrics []metricInfo `json:"metrics"`
}

func (c *Client) listMetrics(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, listMetricsOut, error) {
	return nil, listMetricsOut{Metrics: metricCatalog}, nil
}

type queryIn struct {
	Metric      string `json:"metric" jsonschema:"one of the names returned by list_metrics"`
	From        string `json:"from" jsonschema:"RFC3339 start"`
	To          string `json:"to" jsonschema:"RFC3339 end"`
	EdgeID      string `json:"edge_id,omitempty" jsonschema:"optional edge filter"`
	Method      string `json:"method,omitempty" jsonschema:"optional method filter for route metrics"`
	Path        string `json:"path,omitempty" jsonschema:"optional normalized path filter for route metrics"`
	StepSeconds int    `json:"step_seconds,omitempty" jsonschema:"optional downsampling step in seconds"`
}

type Point struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

type Series struct {
	EdgeID string  `json:"edge_id"`
	Method string  `json:"method,omitempty"`
	Path   string  `json:"path,omitempty"`
	Points []Point `json:"points"`
}

type queryOut struct {
	Metric string    `json:"metric"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Series []Series  `json:"series"`
}

func (c *Client) queryMetric(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, queryOut, error) {
	return c.query(ctx, in)
}

func (c *Client) queryMetricRange(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, queryOut, error) {
	return c.query(ctx, in)
}

func (c *Client) query(ctx context.Context, in queryIn) (*mcp.CallToolResult, queryOut, error) {
	if strings.TrimSpace(in.From) == "" || strings.TrimSpace(in.To) == "" {
		return nil, queryOut{}, fmt.Errorf("from and to are required")
	}
	from, err := time.Parse(time.RFC3339, in.From)
	if err != nil {
		return nil, queryOut{}, fmt.Errorf("from must be RFC3339")
	}
	to, err := time.Parse(time.RFC3339, in.To)
	if err != nil {
		return nil, queryOut{}, fmt.Errorf("to must be RFC3339")
	}
	if !to.After(from) {
		return nil, queryOut{}, fmt.Errorf("to must be after from")
	}
	from, to, err = c.guard.ClampTime(from, to)
	if err != nil {
		return nil, queryOut{}, err
	}
	state, err := c.guard.Load()
	if err != nil {
		return nil, queryOut{}, err
	}
	info, ok := findMetric(in.Metric)
	if !ok {
		return nil, queryOut{}, fmt.Errorf("unknown metric %q", in.Metric)
	}
	if info.Kind == "edge" {
		return c.queryEdge(ctx, info, in, from, to, state)
	}
	return c.queryRoute(ctx, info, in, from, to, state)
}

func findMetric(name string) (metricInfo, bool) {
	for _, info := range metricCatalog {
		if info.Name == name {
			return info, true
		}
	}
	return metricInfo{}, false
}

type edgeRow struct {
	BucketStart time.Time `json:"bucket_start"`
	EdgeID      string    `json:"edge_id"`
	Requests    uint64    `json:"requests"`
	Status4xx   uint64    `json:"status_4xx"`
	Status5xx   uint64    `json:"status_5xx"`
	P95MS       float64   `json:"p95_ms"`
}

func (c *Client) queryEdge(ctx context.Context, info metricInfo, in queryIn, from, to time.Time, state *scope.State) (*mcp.CallToolResult, queryOut, error) {
	if in.EdgeID != "" && state != nil && !state.EdgeAllowed(in.EdgeID) {
		return nil, queryOut{}, scope.ErrDenied
	}
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if in.EdgeID != "" {
		query.Set("edge_id", in.EdgeID)
	}
	var payload struct {
		Rows []edgeRow `json:"rows"`
	}
	if err := c.getJSON(ctx, "/api/metrics/edges?"+query.Encode(), &payload); err != nil {
		return nil, queryOut{}, err
	}
	grouped := map[string][]Point{}
	for _, row := range payload.Rows {
		if state != nil && !state.EdgeAllowed(row.EdgeID) {
			continue
		}
		value := metricValue(info.Name, row.Requests, row.Status4xx, row.Status5xx, row.P95MS)
		grouped[row.EdgeID] = append(grouped[row.EdgeID], Point{At: row.BucketStart.UTC(), Value: value})
	}
	return nil, finishSeries(in.Metric, from, to, grouped, in.StepSeconds), nil
}

type routeRow struct {
	BucketStart time.Time `json:"bucket_start"`
	EdgeID      string    `json:"edge_id"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Requests    uint64    `json:"requests"`
	Status4xx   uint64    `json:"status_4xx"`
	Status5xx   uint64    `json:"status_5xx"`
}

func (c *Client) queryRoute(ctx context.Context, info metricInfo, in queryIn, from, to time.Time, state *scope.State) (*mcp.CallToolResult, queryOut, error) {
	if in.EdgeID != "" && state != nil && !state.EdgeAllowed(in.EdgeID) {
		return nil, queryOut{}, scope.ErrDenied
	}
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if in.EdgeID != "" {
		query.Set("edge_id", in.EdgeID)
	}
	if in.Method != "" {
		query.Set("method", strings.ToUpper(in.Method))
	}
	if in.Path != "" {
		query.Set("path", in.Path)
	}
	var payload struct {
		Rows []routeRow `json:"rows"`
	}
	if err := c.getJSON(ctx, "/api/metrics/routes?"+query.Encode(), &payload); err != nil {
		return nil, queryOut{}, err
	}
	type key struct{ edge, method, path string }
	grouped := map[key][]Point{}
	for _, row := range payload.Rows {
		if state != nil && !state.EdgeAllowed(row.EdgeID) {
			continue
		}
		value := float64(row.Requests)
		if info.Name == MetricRouteErrors {
			value = float64(row.Status4xx + row.Status5xx)
		}
		grouped[key{row.EdgeID, row.Method, row.Path}] = append(grouped[key{row.EdgeID, row.Method, row.Path}], Point{At: row.BucketStart.UTC(), Value: value})
	}
	series := make([]Series, 0, len(grouped))
	for key, points := range grouped {
		series = append(series, Series{EdgeID: key.edge, Method: key.method, Path: key.path, Points: points})
	}
	for index := range series {
		series[index].Points = aggregatePoints(series[index].Points, in.StepSeconds, false)
	}
	sort.Slice(series, func(i, j int) bool {
		if series[i].EdgeID != series[j].EdgeID {
			return series[i].EdgeID < series[j].EdgeID
		}
		if series[i].Method != series[j].Method {
			return series[i].Method < series[j].Method
		}
		return series[i].Path < series[j].Path
	})
	return nil, queryOut{Metric: in.Metric, From: from.UTC(), To: to.UTC(), Series: series}, nil
}

func metricValue(metric string, requests, status4xx, status5xx uint64, p95 float64) float64 {
	switch metric {
	case MetricEdgeRequests, MetricRouteRequests:
		return float64(requests)
	case MetricEdgeErrors, MetricRouteErrors:
		return float64(status4xx + status5xx)
	case MetricEdgeP95:
		return p95
	default:
		return 0
	}
}

func finishSeries(metric string, from, to time.Time, grouped map[string][]Point, stepSeconds int) queryOut {
	series := make([]Series, 0, len(grouped))
	for edgeID, points := range grouped {
		series = append(series, Series{EdgeID: edgeID, Points: aggregatePoints(points, stepSeconds, metric == MetricEdgeP95)})
	}
	sort.Slice(series, func(i, j int) bool { return series[i].EdgeID < series[j].EdgeID })
	return queryOut{Metric: metric, From: from.UTC(), To: to.UTC(), Series: series}
}

func aggregatePoints(points []Point, stepSeconds int, average bool) []Point {
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	if stepSeconds <= 0 || len(points) == 0 {
		return points
	}
	buckets := map[int64][]Point{}
	for _, point := range points {
		key := point.At.Unix() / int64(stepSeconds) * int64(stepSeconds)
		buckets[key] = append(buckets[key], point)
	}
	keys := make([]int64, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]Point, 0, len(keys))
	for _, key := range keys {
		values := buckets[key]
		total := 0.0
		for _, value := range values {
			total += value.Value
		}
		value := total
		if average {
			value = total / float64(len(values))
		}
		result = append(result, Point{At: time.Unix(key, 0).UTC(), Value: value})
	}
	return result
}

type listLabelsIn struct {
	Metric string `json:"metric" jsonschema:"one of the names returned by list_metrics"`
}

type listLabelsOut struct {
	Metric string   `json:"metric"`
	Labels []string `json:"labels"`
}

func (c *Client) listMetricLabels(_ context.Context, _ *mcp.CallToolRequest, in listLabelsIn) (*mcp.CallToolResult, listLabelsOut, error) {
	info, ok := findMetric(in.Metric)
	if !ok {
		return nil, listLabelsOut{}, fmt.Errorf("unknown metric %q", in.Metric)
	}
	return nil, listLabelsOut{Metric: in.Metric, Labels: info.Labels}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("weave api %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(out)
}
