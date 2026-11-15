// Package logs exposes read-only pod logs through kubectl.
package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

type Config struct {
	KubeConfig string
	Namespace  string
	Guard      *scope.Guard
}

type Client struct {
	kubeConfig string
	namespace  string
	guard      *scope.Guard
}

func New(cfg Config) *Client {
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "weave"
	}
	return &Client{kubeConfig: cfg.KubeConfig, namespace: namespace, guard: cfg.Guard}
}

func Register(server *mcp.Server, cfg Config) {
	client := New(cfg)
	mcp.AddTool(server, &mcp.Tool{Name: "list_log_sources", Description: "List workloads and pods whose logs are readable in this run."}, client.listSources)
	mcp.AddTool(server, &mcp.Tool{Name: "query_logs", Description: "Read pod log lines in a time range, optionally the previous container instance."}, client.queryLogs)
}

type Source struct {
	Workload string   `json:"workload"`
	Pods     []string `json:"pods"`
}

func (c *Client) listSources(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []Source, error) {
	workloads, err := c.guard.Workloads()
	if err != nil {
		return nil, nil, err
	}
	if workloads == nil {
		var list podList
		if err := c.kubectlJSON(ctx, &list, "get", "pods", "-o", "json"); err != nil {
			return nil, nil, err
		}
		byWorkload := map[string][]string{}
		for _, pod := range list.Items {
			workload := workloadName(pod.Metadata.Name, pod.Metadata.Labels)
			byWorkload[workload] = append(byWorkload[workload], pod.Metadata.Name)
		}
		result := make([]Source, 0, len(byWorkload))
		for workload, pods := range byWorkload {
			result = append(result, Source{Workload: workload, Pods: pods})
		}
		return nil, result, nil
	}
	result := make([]Source, 0, len(workloads))
	for _, workload := range workloads {
		var list podList
		if err := c.kubectlJSON(ctx, &list, "get", "pods", "-l", "app="+workload, "-o", "json"); err != nil {
			return nil, nil, err
		}
		pods := make([]string, 0, len(list.Items))
		for _, pod := range list.Items {
			pods = append(pods, pod.Metadata.Name)
		}
		result = append(result, Source{Workload: workload, Pods: pods})
	}
	return nil, result, nil
}

type queryIn struct {
	Workload  string `json:"workload,omitempty" jsonschema:"workload name; use either workload or pod"`
	Pod       string `json:"pod,omitempty" jsonschema:"pod name; use either workload or pod"`
	Since     string `json:"since,omitempty" jsonschema:"RFC3339 lower bound; defaults to the incident context start"`
	Until     string `json:"until,omitempty" jsonschema:"RFC3339 upper bound; defaults to the incident context end"`
	Tail      int    `json:"tail,omitempty" jsonschema:"maximum lines per pod; defaults to 200"`
	Filter    string `json:"filter,omitempty" jsonschema:"case-sensitive substring filter"`
	Previous  bool   `json:"previous,omitempty" jsonschema:"read the previous container instance (crash loops)"`
	Container string `json:"container,omitempty" jsonschema:"optional container name"`
}

type Entry struct {
	At        time.Time `json:"at"`
	Pod       string    `json:"pod"`
	Container string    `json:"container,omitempty"`
	Line      string    `json:"line"`
}

type queryOut struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

func (c *Client) queryLogs(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, queryOut, error) {
	if in.Workload == "" && in.Pod == "" {
		return nil, queryOut{}, fmt.Errorf("workload or pod is required")
	}
	if in.Tail <= 0 || in.Tail > 2000 {
		in.Tail = 200
	}
	state, err := c.guard.Load()
	if err != nil {
		return nil, queryOut{}, err
	}
	from, to := time.Time{}, time.Time{}
	if state != nil {
		from, to = state.TimeContext.Start, state.TimeContext.End
	}
	if strings.TrimSpace(in.Since) != "" {
		parsed, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, queryOut{}, fmt.Errorf("since must be RFC3339")
		}
		from = parsed
	}
	if strings.TrimSpace(in.Until) != "" {
		parsed, err := time.Parse(time.RFC3339, in.Until)
		if err != nil {
			return nil, queryOut{}, fmt.Errorf("until must be RFC3339")
		}
		to = parsed
	}
	if !from.IsZero() || !to.IsZero() {
		if from.IsZero() {
			from = time.Now().UTC().Add(-time.Hour)
		}
		if to.IsZero() {
			to = time.Now().UTC()
		}
		from, to, err = c.guard.ClampTime(from, to)
		if err != nil {
			return nil, queryOut{}, err
		}
	}

	pods, err := c.resolvePods(ctx, in, state)
	if err != nil {
		return nil, queryOut{}, err
	}
	out := queryOut{Entries: make([]Entry, 0)}
	for _, pod := range pods {
		args := []string{"logs", pod, "--timestamps"}
		if in.Container != "" {
			args = append(args, "-c", in.Container)
		}
		if in.Previous {
			args = append(args, "--previous")
		}
		if !from.IsZero() {
			args = append(args, "--since-time="+from.UTC().Format(time.RFC3339))
		}
		args = append(args, "--tail="+fmt.Sprintf("%d", in.Tail))
		raw, err := c.kubectl(ctx, args...)
		if err != nil {
			out.Entries = append(out.Entries, Entry{At: time.Now().UTC(), Pod: pod, Line: "[log read failed: " + err.Error() + "]"})
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
			if line == "" {
				continue
			}
			at := time.Time{}
			text := line
			if timestamp, rest, ok := strings.Cut(line, " "); ok {
				if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
					at = parsed
					text = rest
				}
			}
			if in.Filter != "" && !strings.Contains(text, in.Filter) {
				continue
			}
			if !to.IsZero() && !at.IsZero() && at.After(to) {
				continue
			}
			if len(out.Entries) >= 2000 {
				out.Truncated = true
				return nil, out, nil
			}
			out.Entries = append(out.Entries, Entry{At: at, Pod: pod, Container: in.Container, Line: truncate(text, 2000)})
		}
	}
	return nil, out, nil
}

func (c *Client) resolvePods(ctx context.Context, in queryIn, state *scope.State) ([]string, error) {
	if in.Pod != "" {
		var pod podObject
		if err := c.kubectlJSON(ctx, &pod, "get", "pod", in.Pod, "-o", "json"); err != nil {
			return nil, err
		}
		workload := workloadName(pod.Metadata.Name, pod.Metadata.Labels)
		if state != nil && !state.WorkloadAllowed(workload) {
			return nil, scope.ErrDenied
		}
		if in.Workload != "" && workload != in.Workload {
			return nil, fmt.Errorf("pod %q does not belong to workload %q", in.Pod, in.Workload)
		}
		return []string{in.Pod}, nil
	}
	if state != nil && !state.WorkloadAllowed(in.Workload) {
		return nil, scope.ErrDenied
	}
	var list podList
	if err := c.kubectlJSON(ctx, &list, "get", "pods", "-l", "app="+in.Workload, "-o", "json"); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no pods found for workload %q", in.Workload)
	}
	pods := make([]string, 0, len(list.Items))
	for _, pod := range list.Items {
		pods = append(pods, pod.Metadata.Name)
	}
	return pods, nil
}

func (c *Client) kubectlJSON(ctx context.Context, out any, args ...string) error {
	raw, err := c.kubectl(ctx, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) kubectl(ctx context.Context, args ...string) ([]byte, error) {
	base := []string{}
	if c.kubeConfig != "" {
		base = append(base, "--kubeconfig", c.kubeConfig)
	}
	if c.namespace != "" {
		base = append(base, "-n", c.namespace)
	}
	command := exec.CommandContext(ctx, "kubectl", append(base, args...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

type podList struct {
	Items []podObject `json:"items"`
}

type podObject struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
}

func workloadName(name string, labels map[string]string) string {
	if workload := labels["app"]; workload != "" {
		return workload
	}
	if index := strings.Index(name, "-"); index > 0 {
		return name[:index]
	}
	return name
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + " [truncated]"
}
