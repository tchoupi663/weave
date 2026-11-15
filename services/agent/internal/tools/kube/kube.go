// Package kube exposes read-only Kubernetes workload state through kubectl.
package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
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
	mcp.AddTool(server, &mcp.Tool{Name: "list_pods", Description: "List pods for in-scope workloads, or all pods in an unconfined run."}, client.listPods)
	mcp.AddTool(server, &mcp.Tool{Name: "get_pod", Description: "Read one pod's phase, restarts, container states, and labels."}, client.getPod)
	mcp.AddTool(server, &mcp.Tool{Name: "list_deployments", Description: "List deployments for in-scope workloads."}, client.listDeployments)
	mcp.AddTool(server, &mcp.Tool{Name: "get_deployment", Description: "Read one deployment's replicas, images, and conditions."}, client.getDeployment)
	mcp.AddTool(server, &mcp.Tool{Name: "list_statefulsets", Description: "List statefulsets for in-scope workloads."}, client.listStatefulsets)
	mcp.AddTool(server, &mcp.Tool{Name: "get_statefulset", Description: "Read one statefulset's replicas, images, and conditions."}, client.getStatefulset)
	mcp.AddTool(server, &mcp.Tool{Name: "list_events", Description: "List recent Kubernetes events for in-scope workloads."}, client.listEvents)
	mcp.AddTool(server, &mcp.Tool{Name: "describe_workload", Description: "Describe an in-scope workload using kubectl."}, client.describeWorkload)
}

type PodView struct {
	Name      string            `json:"name"`
	Workload  string            `json:"workload"`
	Phase     string            `json:"phase"`
	Restarts  int64             `json:"restarts"`
	StartedAt string            `json:"started_at,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type PodDetail struct {
	PodView
	Containers []ContainerView `json:"containers"`
}

type ContainerView struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int64  `json:"restart_count"`
	State        string `json:"state,omitempty"`
}

type DeploymentView struct {
	Name      string   `json:"name"`
	Workload  string   `json:"workload"`
	Replicas  int      `json:"replicas"`
	Ready     int      `json:"ready"`
	Images    []string `json:"images"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type StatefulSetView struct {
	Name     string   `json:"name"`
	Workload string   `json:"workload"`
	Replicas int      `json:"replicas"`
	Ready    int      `json:"ready"`
	Images   []string `json:"images"`
}

type EventView struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Workload  string `json:"workload,omitempty"`
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
	Count     int    `json:"count"`
}

type emptyIn struct{}

func (c *Client) listPods(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, []PodView, error) {
	workloads, err := c.guard.Workloads()
	if err != nil {
		return nil, nil, err
	}
	if workloads == nil {
		var list podList
		if err := c.kubectlJSON(ctx, &list, "get", "pods", "-o", "json"); err != nil {
			return nil, nil, err
		}
		return nil, convertPods(list.Items), nil
	}
	result := make([]PodView, 0)
	for _, workload := range workloads {
		var list podList
		if err := c.kubectlJSON(ctx, &list, "get", "pods", "-l", "app="+workload, "-o", "json"); err != nil {
			return nil, nil, err
		}
		for _, pod := range convertPods(list.Items) {
			pod.Workload = workload
			result = append(result, pod)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return nil, result, nil
}

type getPodIn struct {
	Pod string `json:"pod" jsonschema:"the pod name"`
}

func (c *Client) getPod(ctx context.Context, _ *mcp.CallToolRequest, in getPodIn) (*mcp.CallToolResult, PodDetail, error) {
	if strings.TrimSpace(in.Pod) == "" {
		return nil, PodDetail{}, fmt.Errorf("pod is required")
	}
	var pod podObject
	if err := c.kubectlJSON(ctx, &pod, "get", "pod", in.Pod, "-o", "json"); err != nil {
		return nil, PodDetail{}, err
	}
	view := convertPod(pod)
	workload := view.Workload
	if workload == "" {
		return nil, PodDetail{}, fmt.Errorf("could not determine the pod's workload")
	}
	if _, err := c.guard.RequireWorkload(workload); err != nil {
		return nil, PodDetail{}, err
	}
	detail := PodDetail{PodView: view}
	for _, status := range pod.Status.ContainerStatuses {
		state := "unknown"
		switch {
		case status.State.Running != nil:
			state = "running"
		case status.State.Waiting != nil:
			state = "waiting: " + status.State.Waiting.Reason
		case status.State.Terminated != nil:
			state = "terminated: " + status.State.Terminated.Reason
		}
		detail.Containers = append(detail.Containers, ContainerView{
			Name:         status.Name,
			Ready:        status.Ready,
			RestartCount: status.RestartCount,
			State:        state,
		})
	}
	return nil, detail, nil
}

func (c *Client) listDeployments(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, []DeploymentView, error) {
	var list deploymentList
	if err := c.kubectlJSON(ctx, &list, "get", "deployments", "-o", "json"); err != nil {
		return nil, nil, err
	}
	workloads, err := c.guard.Workloads()
	if err != nil {
		return nil, nil, err
	}
	result := make([]DeploymentView, 0, len(list.Items))
	for _, item := range list.Items {
		workload := workloadName(item.Metadata.Name, item.Metadata.Labels)
		if workloads != nil && !contains(workloads, workload) {
			continue
		}
		view := DeploymentView{
			Name:     item.Metadata.Name,
			Workload: workload,
			Images:   containerImages(item.Spec.Template.Spec.Containers),
		}
		if item.Spec.Replicas != nil {
			view.Replicas = *item.Spec.Replicas
		}
		if item.Status.ReadyReplicas != nil {
			view.Ready = *item.Status.ReadyReplicas
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return nil, result, nil
}

type getDeploymentIn struct {
	Deployment string `json:"deployment" jsonschema:"the deployment name"`
}

func (c *Client) getDeployment(ctx context.Context, _ *mcp.CallToolRequest, in getDeploymentIn) (*mcp.CallToolResult, DeploymentView, error) {
	if strings.TrimSpace(in.Deployment) == "" {
		return nil, DeploymentView{}, fmt.Errorf("deployment is required")
	}
	if _, err := c.guard.RequireWorkload(in.Deployment); err != nil {
		return nil, DeploymentView{}, err
	}
	var item deploymentObject
	if err := c.kubectlJSON(ctx, &item, "get", "deployment", in.Deployment, "-o", "json"); err != nil {
		return nil, DeploymentView{}, err
	}
	view := DeploymentView{
		Name:      item.Metadata.Name,
		Workload:  workloadName(item.Metadata.Name, item.Metadata.Labels),
		Images:    containerImages(item.Spec.Template.Spec.Containers),
		UpdatedAt: item.Metadata.CreationTimestamp,
	}
	if item.Spec.Replicas != nil {
		view.Replicas = *item.Spec.Replicas
	}
	if item.Status.ReadyReplicas != nil {
		view.Ready = *item.Status.ReadyReplicas
	}
	return nil, view, nil
}

func (c *Client) listStatefulsets(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, []StatefulSetView, error) {
	var list statefulSetList
	if err := c.kubectlJSON(ctx, &list, "get", "statefulsets", "-o", "json"); err != nil {
		return nil, nil, err
	}
	workloads, err := c.guard.Workloads()
	if err != nil {
		return nil, nil, err
	}
	result := make([]StatefulSetView, 0, len(list.Items))
	for _, item := range list.Items {
		workload := workloadName(item.Metadata.Name, item.Metadata.Labels)
		if workloads != nil && !contains(workloads, workload) {
			continue
		}
		view := StatefulSetView{Name: item.Metadata.Name, Workload: workload, Images: containerImages(item.Spec.Template.Spec.Containers)}
		if item.Spec.Replicas != nil {
			view.Replicas = *item.Spec.Replicas
		}
		if item.Status.ReadyReplicas != nil {
			view.Ready = *item.Status.ReadyReplicas
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return nil, result, nil
}

type getStatefulSetIn struct {
	StatefulSet string `json:"statefulset" jsonschema:"the statefulset name"`
}

func (c *Client) getStatefulset(ctx context.Context, _ *mcp.CallToolRequest, in getStatefulSetIn) (*mcp.CallToolResult, StatefulSetView, error) {
	if strings.TrimSpace(in.StatefulSet) == "" {
		return nil, StatefulSetView{}, fmt.Errorf("statefulset is required")
	}
	if _, err := c.guard.RequireWorkload(in.StatefulSet); err != nil {
		return nil, StatefulSetView{}, err
	}
	var item statefulSetObject
	if err := c.kubectlJSON(ctx, &item, "get", "statefulset", in.StatefulSet, "-o", "json"); err != nil {
		return nil, StatefulSetView{}, err
	}
	view := StatefulSetView{Name: item.Metadata.Name, Workload: workloadName(item.Metadata.Name, item.Metadata.Labels), Images: containerImages(item.Spec.Template.Spec.Containers)}
	if item.Spec.Replicas != nil {
		view.Replicas = *item.Spec.Replicas
	}
	if item.Status.ReadyReplicas != nil {
		view.Ready = *item.Status.ReadyReplicas
	}
	return nil, view, nil
}

type listEventsIn struct {
	Since string `json:"since,omitempty" jsonschema:"optional RFC3339 lower bound"`
}

func (c *Client) listEvents(ctx context.Context, _ *mcp.CallToolRequest, in listEventsIn) (*mcp.CallToolResult, []EventView, error) {
	var since time.Time
	if strings.TrimSpace(in.Since) != "" {
		parsed, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("since must be RFC3339")
		}
		since = parsed
	}
	var list eventList
	if err := c.kubectlJSON(ctx, &list, "get", "events", "--sort-by=.lastTimestamp", "-o", "json"); err != nil {
		return nil, nil, err
	}
	workloads, err := c.guard.Workloads()
	if err != nil {
		return nil, nil, err
	}
	result := make([]EventView, 0, len(list.Items))
	for _, item := range list.Items {
		workload := eventWorkload(item)
		if workloads != nil && (workload == "" || !contains(workloads, workload)) {
			continue
		}
		if !since.IsZero() {
			if last := parseKubeTime(item.LastTimestamp); !last.IsZero() && last.Before(since) {
				continue
			}
		}
		result = append(result, EventView{
			Type: item.Type, Reason: item.Reason, Message: item.Message, Workload: workload,
			FirstSeen: item.FirstTimestamp, LastSeen: item.LastTimestamp, Count: item.Count,
		})
	}
	return nil, result, nil
}

type describeIn struct {
	Workload string `json:"workload" jsonschema:"the in-scope workload to describe"`
}

type describeOut struct {
	Workload string `json:"workload"`
	Kind     string `json:"kind"`
	Output   string `json:"output"`
}

func (c *Client) describeWorkload(ctx context.Context, _ *mcp.CallToolRequest, in describeIn) (*mcp.CallToolResult, describeOut, error) {
	if strings.TrimSpace(in.Workload) == "" {
		return nil, describeOut{}, fmt.Errorf("workload is required")
	}
	if _, err := c.guard.RequireWorkload(in.Workload); err != nil {
		return nil, describeOut{}, err
	}
	for _, kind := range []string{"deployment", "statefulset", "daemonset"} {
		output, err := c.kubectl(ctx, "describe", kind, in.Workload)
		if err == nil {
			return nil, describeOut{Workload: in.Workload, Kind: kind, Output: truncate(string(output), 16000)}, nil
		}
	}
	return nil, describeOut{}, fmt.Errorf("workload %q does not exist or is not readable", in.Workload)
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
	Metadata objectMeta `json:"metadata"`
	Status   struct {
		Phase             string            `json:"phase"`
		StartTime         string            `json:"startTime"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type objectMeta struct {
	Name              string            `json:"name"`
	Labels            map[string]string `json:"labels"`
	CreationTimestamp string            `json:"creationTimestamp"`
}

type containerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int64  `json:"restartCount"`
	State        struct {
		Running *struct{} `json:"running"`
		Waiting *struct {
			Reason string `json:"reason"`
		} `json:"waiting"`
		Terminated *struct {
			Reason string `json:"reason"`
		} `json:"terminated"`
	} `json:"state"`
}

type deploymentList struct {
	Items []deploymentObject `json:"items"`
}

type deploymentObject struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []containerSpec `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas *int `json:"readyReplicas"`
	} `json:"status"`
}

type statefulSetList struct {
	Items []statefulSetObject `json:"items"`
}

type statefulSetObject struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []containerSpec `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas *int `json:"readyReplicas"`
	} `json:"status"`
}

type containerSpec struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type eventList struct {
	Items []eventObject `json:"items"`
}

type eventObject struct {
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	InvolvedObject struct {
		Kind   string            `json:"kind"`
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"involvedObject"`
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
	Count          int    `json:"count"`
}

func convertPods(items []podObject) []PodView {
	result := make([]PodView, 0, len(items))
	for _, item := range items {
		result = append(result, convertPod(item))
	}
	return result
}

func convertPod(item podObject) PodView {
	view := PodView{
		Name:      item.Metadata.Name,
		Workload:  workloadName(item.Metadata.Name, item.Metadata.Labels),
		Phase:     item.Status.Phase,
		StartedAt: item.Status.StartTime,
		Labels:    item.Metadata.Labels,
	}
	for _, status := range item.Status.ContainerStatuses {
		view.Restarts += status.RestartCount
	}
	return view
}

func workloadName(name string, labels map[string]string) string {
	if workload := labels["app"]; workload != "" {
		return workload
	}
	for _, suffix := range []string{"-"} {
		if index := strings.LastIndex(name, suffix); index > 0 {
			return name[:index]
		}
	}
	return name
}

func eventWorkload(item eventObject) string {
	if workload := item.InvolvedObject.Labels["app"]; workload != "" {
		return workload
	}
	name := item.InvolvedObject.Name
	if index := strings.Index(name, "-"); index > 0 {
		return name[:index]
	}
	return name
}

func containerImages(containers []containerSpec) []string {
	result := make([]string, 0, len(containers))
	for _, container := range containers {
		result = append(result, container.Image)
	}
	return result
}

func parseKubeTime(raw string) time.Time {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n[truncated]"
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
