package topology

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	flowpb "github.com/cilium/cilium/api/v1/flow"
)

type endpointRef struct {
	ID        string
	Name      string
	Workload  string
	Kind      string
	Namespace string
	Pod       string
	IP        string
}

type mappedFlow struct {
	Source     endpointRef
	Target     endpointRef
	Protocol   string
	Port       uint32
	Connection bool
	EventKey   string
	Time       time.Time
}

func mapFlow(f *flowpb.Flow, watchNamespace string) (mappedFlow, bool) {
	if f == nil || f.GetType() != flowpb.FlowType_L3_L4 {
		return mappedFlow{}, false
	}
	if f.GetVerdict() != flowpb.Verdict_FORWARDED {
		return mappedFlow{}, false
	}
	if flowIsReply(f) {
		return mappedFlow{}, false
	}

	src := f.GetSource()
	dst := f.GetDestination()
	if src == nil || dst == nil {
		return mappedFlow{}, false
	}

	port, syn, ok := destinationPort(f)
	if !ok {
		return mappedFlow{}, false
	}

	target, ok := workloadEndpoint(dst, watchNamespace, f.GetIP().GetDestination())
	if !ok {
		return mappedFlow{}, false
	}

	source, external, ok := sourceEndpoint(src, f.GetIP().GetSource(), watchNamespace)
	if !ok {
		return mappedFlow{}, false
	}
	if external {
		// An external endpoint is only an ingress point when it enters the
		// exposed Frontend. Host traffic to any other workload is a kubelet
		// probe or control-plane traffic, not application topology.
		if target.Workload != "frontend" || port != 443 {
			return mappedFlow{}, false
		}
	}
	if source.ID == target.ID {
		return mappedFlow{}, false
	}

	var t time.Time
	if ts := f.GetTime(); ts != nil {
		t = ts.AsTime()
	}
	if t.IsZero() {
		t = time.Now()
	}

	eventKey := f.GetUuid()
	if eventKey == "" {
		eventKey = fmt.Sprintf("%s|%s|%s|%d|%d", source.ID, target.ID, f.GetIP().GetSource(), port, t.UnixNano())
	}

	return mappedFlow{
		Source:     source,
		Target:     target,
		Protocol:   protocolFor(dst, port, f.GetL4().GetUDP() != nil),
		Port:       port,
		Connection: syn,
		EventKey:   eventKey,
		Time:       t,
	}, true
}

func flowIsReply(f *flowpb.Flow) bool {
	if v := f.GetIsReply(); v != nil {
		return v.GetValue()
	}
	return f.GetReply()
}

func destinationPort(f *flowpb.Flow) (uint32, bool, bool) {
	if tcp := f.GetL4().GetTCP(); tcp != nil {
		flags := tcp.GetFlags()
		return tcp.GetDestinationPort(), flags.GetSYN() && !flags.GetACK(), true
	}
	if udp := f.GetL4().GetUDP(); udp != nil {
		return udp.GetDestinationPort(), false, true
	}
	return 0, false, false
}

func workloadEndpoint(e *flowpb.Endpoint, watchNamespace, ip string) (endpointRef, bool) {
	if e.GetNamespace() != watchNamespace {
		return endpointRef{}, false
	}
	workload, kind := workloadIdentity(e)
	if workload == "" {
		return endpointRef{}, false
	}
	return endpointRef{
		ID:        "workload:" + watchNamespace + "/" + workload,
		Name:      displayName(workload),
		Workload:  workload,
		Kind:      kind,
		Namespace: watchNamespace,
		Pod:       e.GetPodName(),
		IP:        ip,
	}, true
}

func sourceEndpoint(e *flowpb.Endpoint, ip, watchNamespace string) (endpointRef, bool, bool) {
	if e.GetNamespace() == watchNamespace {
		ref, ok := workloadEndpoint(e, watchNamespace, ip)
		return ref, false, ok
	}
	if isExternalEndpoint(e) {
		if ip == "" {
			ip = "unknown"
		}
		return endpointRef{
			ID:       "source-ip",
			Name:     "Source IP",
			Workload: "source-ip",
			Kind:     "External Client",
			IP:       ip,
		}, true, true
	}
	return endpointRef{}, false, false
}

func isExternalEndpoint(e *flowpb.Endpoint) bool {
	if e.GetNamespace() != "" || e.GetPodName() != "" {
		return false
	}
	for _, label := range e.GetLabels() {
		switch label {
		case "reserved:host", "reserved:world", "reserved:remote-node":
			return true
		}
	}
	return false
}

func workloadIdentity(e *flowpb.Endpoint) (string, string) {
	for _, w := range e.GetWorkloads() {
		if name := strings.TrimSpace(w.GetName()); name != "" {
			kind := w.GetKind()
			if kind == "" {
				kind = "Workload"
			}
			return name, kind
		}
	}
	for _, label := range e.GetLabels() {
		if strings.HasPrefix(label, "k8s:app=") {
			name := strings.TrimPrefix(label, "k8s:app=")
			if name != "" {
				return name, "Workload"
			}
		}
	}
	pod := e.GetPodName()
	if pod == "" {
		return "", ""
	}
	// A pod name ends in a ReplicaSet hash and a five-character suffix. This
	// fallback only runs when Hubble did not attach workload metadata.
	parts := strings.Split(pod, "-")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-2], "-"), "Workload"
	}
	return pod, "Workload"
}

func protocolFor(dst *flowpb.Endpoint, port uint32, udp bool) string {
	if name := namedPort(dst.GetLabels(), port); name != "" {
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "https"):
			return "HTTPS"
		case strings.Contains(lower, "http"):
			return "HTTP"
		case strings.Contains(lower, "grpc"):
			return "gRPC"
		case strings.Contains(lower, "postgres"):
			return "TCP"
		case strings.Contains(lower, "dns"):
			return "DNS"
		}
	}
	if udp {
		return "UDP"
	}
	switch port {
	case 443, 8443:
		return "HTTPS"
	case 80, 8000, 8080, 8081, 8082, 8083, 8084, 8085, 8086, 8087, 8088, 8089:
		return "HTTP"
	default:
		return "TCP"
	}
}

func namedPort(labels []string, port uint32) string {
	target := strconv.FormatUint(uint64(port), 10)
	for _, label := range labels {
		const prefix = "gen:io.cilium.k8s.named-ports="
		if !strings.HasPrefix(label, prefix) {
			continue
		}
		spec := strings.TrimPrefix(label, prefix)
		for _, entry := range strings.Split(spec, "_") {
			parts := strings.Split(entry, ".")
			if len(parts) < 3 {
				continue
			}
			if parts[len(parts)-1] == target {
				return parts[0]
			}
		}
	}
	return ""
}
