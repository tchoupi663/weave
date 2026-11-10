package api

import (
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/topology"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const maxOTLPBody = 8 << 20

func (s *Server) traces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOTLPBody))
	if err != nil {
		http.Error(w, "trace payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	var request collectortrace.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid OTLP trace payload", http.StatusBadRequest)
		return
	}

	accepted := 0
	for _, resourceSpans := range request.GetResourceSpans() {
		resource := attributeMap(resourceSpans.GetResource().GetAttributes())
		namespace := resource["k8s.namespace.name"]
		if namespace != s.store.Namespace() {
			continue
		}
		source := sourceWorkload(resource)
		if source == "" {
			continue
		}
		pod := resource["k8s.pod.name"]
		kind := "Workload"
		if resource["k8s.deployment.name"] != "" {
			kind = "Deployment"
		}

		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				var observation topology.L7Observation
				var ok bool
				switch span.GetKind() {
				case tracepb.Span_SPAN_KIND_CLIENT:
					observation, ok = clientObservation(span, source, kind, pod, namespace)
				case tracepb.Span_SPAN_KIND_SERVER:
					observation, ok = s.serverObservation(span, source, kind, pod, namespace)
				}
				if ok && s.store.IngestL7(observation) {
					accepted++
				}
			}
		}
	}
	if accepted > 0 {
		s.log.Debug("ingested HTTP observations", "accepted", accepted)
	}

	response, _ := proto.Marshal(&collectortrace.ExportTraceServiceResponse{})
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

func clientObservation(span *tracepb.Span, source, kind, pod, namespace string) (topology.L7Observation, bool) {
	attributes := attributeMap(span.GetAttributes())
	host := firstString(attributes, "server.address", "net.peer.name", "peer.service", "http.host")
	target, targetIP := targetHost(host)
	port := firstPort(attributes, "server.port", "net.peer.port")
	if port == 0 {
		port = portFromURL(attributes)
	}
	if port == 0 {
		return topology.L7Observation{}, false
	}

	method := firstString(attributes, "http.request.method", "http.method")
	path := firstString(attributes, "url.path", "http.target")
	if path == "" {
		path = pathFromURL(firstString(attributes, "url.full", "http.url"))
	}
	if method == "" || path == "" {
		return topology.L7Observation{}, false
	}

	start := span.GetStartTimeUnixNano()
	end := span.GetEndTimeUnixNano()
	if start == 0 || end <= start {
		return topology.L7Observation{}, false
	}

	status := uint32(firstInt(attributes, "http.response.status_code", "http.status_code"))
	key := "trace:" + hex.EncodeToString(span.GetTraceId()) + ":" + hex.EncodeToString(span.GetSpanId())
	if len(span.GetTraceId()) == 0 && len(span.GetSpanId()) == 0 {
		key = ""
	}

	return topology.L7Observation{
		SourceWorkload:  source,
		SourceKind:      kind,
		SourcePod:       pod,
		SourceNamespace: namespace,
		TargetWorkload:  target,
		TargetNamespace: namespace,
		TargetIP:        targetIP,
		Port:            port,
		Method:          method,
		Path:            path,
		StatusCode:      status,
		LatencySource:   "client",
		Duration:        time.Duration(end - start),
		Time:            time.Unix(0, int64(start)).UTC(),
		Key:             key,
	}, true
}

func (s *Server) serverObservation(span *tracepb.Span, target, kind, pod, namespace string) (topology.L7Observation, bool) {
	attributes := attributeMap(span.GetAttributes())
	peer := firstString(attributes, "client.address", "net.peer.name", "peer.service", "peer.address")
	if peer == "" {
		return topology.L7Observation{}, false
	}
	source, sourceIP := targetHost(peer)
	if source == "" && sourceIP != "" {
		resolved, ok := s.store.ResolveWorkloadByIP(sourceIP)
		if !ok {
			return topology.L7Observation{}, false
		}
		source = resolved
	}
	if source == "" || source == target {
		return topology.L7Observation{}, false
	}
	port := firstPort(attributes, "server.port", "net.peer.port")
	if port == 0 {
		port = portFromURL(attributes)
	}
	if port == 0 {
		return topology.L7Observation{}, false
	}
	method := firstString(attributes, "http.request.method", "http.method")
	path := firstString(attributes, "url.path", "http.target")
	if path == "" {
		path = pathFromURL(firstString(attributes, "url.full", "http.url"))
	}
	if method == "" || path == "" {
		return topology.L7Observation{}, false
	}
	start := span.GetStartTimeUnixNano()
	end := span.GetEndTimeUnixNano()
	if start == 0 || end <= start {
		return topology.L7Observation{}, false
	}
	status := uint32(firstInt(attributes, "http.response.status_code", "http.status_code"))
	key := "trace:" + hex.EncodeToString(span.GetTraceId()) + ":" + hex.EncodeToString(span.GetSpanId())
	if len(span.GetTraceId()) == 0 && len(span.GetSpanId()) == 0 {
		key = ""
	}
	return topology.L7Observation{
		SourceWorkload:  source,
		SourceKind:      "Workload",
		SourceNamespace: namespace,
		TargetWorkload:  target,
		TargetNamespace: namespace,
		Port:            port,
		Method:          method,
		Path:            path,
		StatusCode:      status,
		LatencySource:   "server",
		Duration:        time.Duration(end - start),
		Time:            time.Unix(0, int64(start)).UTC(),
		Key:             key,
	}, true
}

func sourceWorkload(resource map[string]string) string {
	if value := resource["k8s.deployment.name"]; value != "" {
		return value
	}
	if value := resource["k8s.statefulset.name"]; value != "" {
		return value
	}
	if value := resource["service.name"]; value != "" {
		return value
	}
	pod := resource["k8s.pod.name"]
	parts := strings.Split(pod, "-")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return pod
}

func targetHost(host string) (string, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", ""
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.TrimSuffix(host, ".")
	if net.ParseIP(host) != nil {
		return "", host
	}
	if host == "localhost" {
		return "", ""
	}
	if index := strings.IndexByte(host, '.'); index > 0 {
		host = host[:index]
	}
	return host, ""
}

func portFromURL(attributes map[string]string) uint32 {
	raw := firstString(attributes, "url.full", "http.url")
	if raw == "" {
		return 0
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	return portFromString(parsed.Port())
}

func pathFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Path
}

func attributeMap(values []*commonpb.KeyValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		if value == nil || value.GetKey() == "" {
			continue
		}
		switch typed := value.GetValue().GetValue().(type) {
		case *commonpb.AnyValue_StringValue:
			result[value.GetKey()] = typed.StringValue
		case *commonpb.AnyValue_IntValue:
			result[value.GetKey()] = strconv.FormatInt(typed.IntValue, 10)
		case *commonpb.AnyValue_BoolValue:
			result[value.GetKey()] = strconv.FormatBool(typed.BoolValue)
		case *commonpb.AnyValue_DoubleValue:
			result[value.GetKey()] = strconv.FormatFloat(typed.DoubleValue, 'f', -1, 64)
		}
	}
	return result
}

func firstString(attributes map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := attributes[key]; value != "" {
			return value
		}
	}
	return ""
}

func firstInt(attributes map[string]string, keys ...string) int64 {
	for _, key := range keys {
		if value := attributes[key]; value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func firstPort(attributes map[string]string, keys ...string) uint32 {
	return portFromString(firstString(attributes, keys...))
}

func portFromString(raw string) uint32 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(value)
}
