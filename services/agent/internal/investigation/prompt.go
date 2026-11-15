package investigation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PromptVersion is recorded with every run.
const PromptVersion = "investigator-v1"

func SystemPrompt(arm string) string {
	base := `You are Weave Investigator, a read-only root-cause investigator for a Kubernetes application.
You never change the cluster, the application, or Weave. You have no remediation tools.
Your job is to find the root cause of the given incident and return a single JSON object, nothing else.

Evidence rules:
- Distinguish symptoms from causes. The caller's 5xx is not automatically the root cause.
- Use the observed topology, metrics, logs, Kubernetes state, and source code.
- Treat missing recording as missing data, not calm traffic.
- Be explicit when evidence is insufficient.
- Query metrics with a concrete edge_id or route filter whenever one is known; do not request all series repeatedly.
- Do not repeat a tool call with identical arguments. If a query returns no new information, change the tool or direction.
- Stay inside the initial region while its evidence is sufficient. If it is not, prefer expanding downstream from the workload whose requests are failing.

Return exactly this JSON shape:
{
  "root_cause": {"workload": "<lower-case workload name>", "failure_mode": "http_error|latency|dependency_unavailable|bad_deployment|resource_saturation|network_fault|other"},
  "affected": {"edge_id": "<observed edge id, optional>", "route": "<METHOD /normalized/path, optional>"},
  "evidence": [{"source": "metrics|logs|kube|source|weave", "reference": "<tool or metric reference>", "observation": "<short observation>"}],
  "confidence": 0.0,
  "summary": "<one or two sentences>"
}
Do not wrap the JSON in Markdown fences.`
	if arm == ArmConfined {
		return base + `

You are confined to a Region Scope. The initial context below is your entire world:
you cannot read topology, metrics, logs, Kubernetes objects, changes, or source for anything outside it.
The only exception is an explicit scope expansion. A node listed as a frontier has hidden
observed neighbours. If and only if the in-region evidence indicates the root cause is outside
the scope, call the weave__expand_scope tool with the frontier node, a direction
(upstream/downstream/both), and a rationale. Expansion adds only the immediate neighbours;
repeat only when necessary. Prefer staying inside the region when the evidence is sufficient.`
	}
	return base + `

You have access to the full observed architecture for this incident, through the same tools.
Investigate as broadly as the evidence requires.`
}

func UserPrompt(incident Incident, regionJSON []byte) string {
	var builder strings.Builder
	builder.WriteString("Incident ")
	builder.WriteString(incident.ID)
	if incident.Status != "" {
		builder.WriteString(" (" + incident.Status + ")")
	}
	builder.WriteString("\n")
	if !incident.WindowStart.IsZero() {
		builder.WriteString(fmt.Sprintf("Window: %s to %s\n", incident.WindowStart.UTC().Format("2006-01-02T15:04:05Z"), incident.WindowEnd.UTC().Format("2006-01-02T15:04:05Z")))
	}
	if incident.Symptom != "" {
		builder.WriteString("Symptom: " + incident.Symptom + "\n")
	}
	if len(incident.Trigger) > 0 {
		var trigger any
		if json.Unmarshal(incident.Trigger, &trigger) == nil {
			raw, _ := json.Marshal(trigger)
			builder.WriteString("Detection trigger: " + string(raw) + "\n")
		}
	}
	if len(regionJSON) > 0 {
		builder.WriteString("\nBounded region context (your starting scope):\n")
		builder.Write(regionJSON)
		builder.WriteString("\n")
	}
	builder.WriteString("\nInvestigate and return the diagnosis JSON.")
	return builder.String()
}
